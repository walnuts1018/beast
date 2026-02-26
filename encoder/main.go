package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/walnuts1018/beast/encoder/internal/config"
	"github.com/walnuts1018/beast/encoder/internal/logger"
	"github.com/walnuts1018/beast/encoder/internal/rabbitmq"
	"github.com/walnuts1018/beast/encoder/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "config load failed", slog.Any("error", err))
		os.Exit(1)
	}

	appLogger := logger.New(cfg.LogLevel, cfg.LogType)
	slog.SetDefault(appLogger)

	rmqClient, err := rabbitmq.New(cfg.RabbitMQURL, cfg.EncodeJobQueue, cfg.EncodeEventQueue, cfg.ConsumerTag)
	if err != nil {
		slog.ErrorContext(ctx, "rabbitmq initialization failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer rmqClient.Close()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.S3Region))
	if err != nil {
		slog.ErrorContext(ctx, "aws config load failed", slog.Any("error", err))
		os.Exit(1)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.S3UsePathStyle
		o.BaseEndpoint = aws.String(cfg.S3Endpoint)
	})

	if _, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.S3Bucket)}); err != nil {
		slog.ErrorContext(ctx, "s3 bucket check failed", slog.Any("error", err))
		os.Exit(1)
	}

	w := worker.New(
		rmqClient,
		s3Client,
		cfg.S3Bucket,
		cfg.WorkDir,
		cfg.FFmpegPath,
		cfg.FFprobePath,
		cfg.DashSegmentSeconds,
		cfg.JobTimeout,
		appLogger,
	)

	slog.InfoContext(ctx, "encoder worker started")
	if err := w.Run(ctx); err != nil {
		slog.ErrorContext(ctx, "worker stopped", slog.Any("error", err))
		os.Exit(1)
	}
}
