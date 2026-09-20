package main

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/walnuts1018/beast/encoder/internal/config"
	"github.com/walnuts1018/beast/encoder/internal/queue"
	"github.com/walnuts1018/beast/encoder/internal/storage"
	"github.com/walnuts1018/beast/encoder/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("encoder stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	runner := &worker.FFmpegRunner{
		FFmpegPath:            cfg.FFmpegPath,
		FFprobePath:           cfg.FFprobePath,
		SegmentSeconds:        cfg.SegmentSeconds,
		ReencodeOnCopyFailure: cfg.ReencodeOnCopyFailure,
		Logger:                logger,
	}
	objectStore, err := storage.NewS3(ctx, cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.StagingEncryptionKey, cfg.MediaEncryptionKey)
	if err != nil {
		return err
	}
	if err := objectStore.Check(ctx); err != nil {
		return err
	}

	if cfg.JobInput == "stdin" {
		return runStdin(ctx, cfg, runner, objectStore, logger)
	}
	client, deliveries, err := queue.New(cfg.RabbitMQURL, cfg.JobQueue, cfg.EventQueue, cfg.ConsumerTag)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			logger.Error("close RabbitMQ failed", "error", closeErr)
		}
	}()
	processor := &worker.Processor{Runner: runner, Store: objectStore, OutputRoot: cfg.OutputRoot, Emit: client.Publish}
	logger.Info("encoder worker started", "job_queue", cfg.JobQueue, "event_queue", cfg.EventQueue)
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("RabbitMQ delivery channel closed")
			}
			var job worker.Job
			if err := json.Unmarshal(delivery.Body, &job); err != nil {
				_ = delivery.Nack(false, false)
				logger.Error("decode encoder job failed", "error", err)
				continue
			}
			if err := processor.Process(ctx, job); err != nil {
				logger.Error("encoder job failed", "video_id", job.VideoID, "error", err)
				if err := delivery.Nack(false, true); err != nil {
					return fmt.Errorf("requeue encoder job: %w", err)
				}
				continue
			}
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("ack encoder job: %w", err)
			}
		}
	}
}

func runStdin(ctx context.Context, cfg config.Config, runner *worker.FFmpegRunner, objectStore *storage.S3, logger *slog.Logger) error {
	var events []worker.Event
	emit := func(_ context.Context, event worker.Event) error {
		events = append(events, event)
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		body = append(body, '\n')
		_, err = os.Stdout.Write(body)
		return err
	}
	processor := &worker.Processor{Runner: runner, Store: objectStore, OutputRoot: cfg.OutputRoot, Emit: emit}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		var job worker.Job
		if err := json.Unmarshal(scanner.Bytes(), &job); err != nil {
			return fmt.Errorf("decode encoder job: %w", err)
		}
		if err := processor.Process(ctx, job); err != nil {
			logger.Error("encoder job failed", "video_id", job.VideoID, "error", err)
		}
	}
	return scanner.Err()
}
