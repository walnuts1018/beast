package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/walnuts1018/beast/encoder/internal/rabbitmq"
)

type Worker struct {
	rmq                *rabbitmq.Client
	s3Client           *s3.Client
	bucket             string
	workDir            string
	ffmpegPath         string
	ffprobePath        string
	dashSegmentSeconds int
	logger             *slog.Logger
}

type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     *int   `json:"width"`
	Height    *int   `json:"height"`
}

type probeFormat struct {
	Duration string `json:"duration"`
}

type mediaMeta struct {
	VideoCodec     string
	AudioCodec     string
	DurationMillis *int
	Width          *int
	Height         *int
}

func New(
	rmq *rabbitmq.Client,
	s3Client *s3.Client,
	bucket string,
	workDir string,
	ffmpegPath string,
	ffprobePath string,
	dashSegmentSeconds int,
	logger *slog.Logger,
) *Worker {
	if workDir == "" {
		workDir = "/tmp/beast-encoder"
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	if dashSegmentSeconds <= 0 {
		dashSegmentSeconds = 4
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Worker{
		rmq:                rmq,
		s3Client:           s3Client,
		bucket:             bucket,
		workDir:            workDir,
		ffmpegPath:         ffmpegPath,
		ffprobePath:        ffprobePath,
		dashSegmentSeconds: dashSegmentSeconds,
		logger:             logger,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	deliveries, err := w.rmq.ConsumeJobs(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}

			job, err := w.rmq.DecodeJob(delivery.Body)
			if err != nil {
				w.logger.ErrorContext(ctx, "failed to decode encode job", slog.Any("error", err))
				_ = delivery.Nack(false, false)
				continue
			}

			if err := w.publishProgress(ctx, job, 1, nil); err != nil {
				w.logger.ErrorContext(ctx, "failed to publish progress", slog.Any("error", err), slog.String("videoID", job.VideoID))
				_ = delivery.Nack(false, true)
				continue
			}

			if err := w.encodeDash(ctx, job); err != nil {
				reason := err.Error()
				if pubErr := w.publishFailed(ctx, job, reason); pubErr != nil {
					w.logger.ErrorContext(ctx, "failed to publish failure event", slog.Any("error", pubErr), slog.String("videoID", job.VideoID))
					_ = delivery.Nack(false, true)
					continue
				}
				_ = delivery.Ack(false)
				continue
			}

			if err := delivery.Ack(false); err != nil {
				w.logger.ErrorContext(ctx, "failed to ack encode job", slog.Any("error", err), slog.String("videoID", job.VideoID))
			}
		}
	}
}

func (w *Worker) encodeDash(ctx context.Context, job rabbitmq.EncodeJobMessage) error {
	if err := os.MkdirAll(w.workDir, 0o755); err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(w.workDir, fmt.Sprintf("dash-%s-*", job.VideoID))
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input")
	outputDir := filepath.Join(tmpDir, "dash")
	manifestPath := filepath.Join(outputDir, "stream.mpd")

	if err := w.downloadSource(ctx, job.SourceObjectKey, inputPath); err != nil {
		return err
	}

	meta, err := w.probeInput(ctx, inputPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create dash dir: %w", err)
	}

	if err := w.runFFmpegDash(ctx, inputPath, manifestPath, meta); err != nil {
		return err
	}

	if err := w.publishProgress(ctx, job, 80, nil); err != nil {
		return err
	}

	prefix := fmt.Sprintf("videos/%s/%s/dash/", job.OwnerUserID, job.VideoID)
	if err := w.uploadDashDirectory(ctx, outputDir, prefix); err != nil {
		return err
	}

	manifest := prefix + "stream.mpd"
	msg := "dash encoding completed"
	event := rabbitmq.EncodeEventMessage{
		Type:              "completed",
		VideoID:           job.VideoID,
		OwnerUserID:       job.OwnerUserID,
		Percent:           float64Ptr(100),
		Message:           &msg,
		ManifestObjectKey: &manifest,
		EncodedObjectKey:  &prefix,
		DurationMillis:    meta.DurationMillis,
		Width:             meta.Width,
		Height:            meta.Height,
	}
	if err := w.rmq.PublishEvent(ctx, event); err != nil {
		return fmt.Errorf("publish completed event: %w", err)
	}

	w.logger.InfoContext(ctx, "dash encoding completed", slog.String("videoID", job.VideoID), slog.String("manifest", manifest))
	return nil
}

func (w *Worker) publishProgress(ctx context.Context, job rabbitmq.EncodeJobMessage, percent float64, message *string) error {
	event := rabbitmq.EncodeEventMessage{
		Type:        "progress",
		VideoID:     job.VideoID,
		OwnerUserID: job.OwnerUserID,
		Percent:     &percent,
		Message:     message,
	}
	if err := w.rmq.PublishEvent(ctx, event); err != nil {
		return fmt.Errorf("publish progress event: %w", err)
	}
	return nil
}

func (w *Worker) publishFailed(ctx context.Context, job rabbitmq.EncodeJobMessage, reason string) error {
	event := rabbitmq.EncodeEventMessage{
		Type:        "failed",
		VideoID:     job.VideoID,
		OwnerUserID: job.OwnerUserID,
		Percent:     float64Ptr(100),
		Message:     &reason,
	}
	if err := w.rmq.PublishEvent(ctx, event); err != nil {
		return fmt.Errorf("publish failed event: %w", err)
	}
	return nil
}

func (w *Worker) runFFmpegDash(ctx context.Context, inputPath string, manifestPath string, meta mediaMeta) error {
	videoCopy := strings.EqualFold(meta.VideoCodec, "h264")
	audioCopy := meta.AudioCodec == "" || strings.EqualFold(meta.AudioCodec, "aac")

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a?",
	}

	if videoCopy {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23")
	}

	if audioCopy {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}

	args = append(args,
		"-f", "dash",
		"-seg_duration", strconv.Itoa(w.dashSegmentSeconds),
		"-use_timeline", "1",
		"-use_template", "1",
		"-window_size", "5",
		"-extra_window_size", "5",
		"-adaptation_sets", "id=0,streams=v id=1,streams=a",
		manifestPath,
	)

	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg dash failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func (w *Worker) probeInput(ctx context.Context, inputPath string) (mediaMeta, error) {
	cmd := exec.CommandContext(
		ctx,
		w.ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return mediaMeta{}, fmt.Errorf("ffprobe failed: %w", err)
	}

	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return mediaMeta{}, fmt.Errorf("decode ffprobe json: %w", err)
	}

	var videoCodec string
	var audioCodec string
	var width *int
	var height *int
	for _, stream := range result.Streams {
		if stream.CodecType == "video" && videoCodec == "" {
			videoCodec = stream.CodecName
			width = stream.Width
			height = stream.Height
		}
		if stream.CodecType == "audio" && audioCodec == "" {
			audioCodec = stream.CodecName
		}
	}

	var durationMillis *int
	if result.Format.Duration != "" {
		seconds, err := strconv.ParseFloat(result.Format.Duration, 64)
		if err != nil {
			return mediaMeta{}, fmt.Errorf("parse duration: %w", err)
		}
		d := int(seconds * 1000)
		durationMillis = &d
	}

	if videoCodec == "" {
		return mediaMeta{}, fmt.Errorf("video stream not found")
	}

	return mediaMeta{VideoCodec: videoCodec, AudioCodec: audioCodec, DurationMillis: durationMillis, Width: width, Height: height}, nil
}

func (w *Worker) uploadDashDirectory(ctx context.Context, dir string, prefix string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		objectKey := prefix + filepath.ToSlash(rel)

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open dash artifact: %w", err)
		}
		defer f.Close()

		contentType := contentTypeByPath(path)
		_, err = w.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(w.bucket),
			Key:         aws.String(objectKey),
			Body:        f,
			ContentType: aws.String(contentType),
		})
		if err != nil {
			return fmt.Errorf("upload dash artifact: %w", err)
		}
		return nil
	})
}

func contentTypeByPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mpd":
		return "application/dash+xml"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	default:
		if v := mime.TypeByExtension(ext); v != "" {
			return v
		}
		return "application/octet-stream"
	}
}

func (w *Worker) downloadSource(ctx context.Context, objectKey string, outputPath string) error {
	resp, err := w.s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(w.bucket), Key: aws.String(objectKey)})
	if err != nil {
		return fmt.Errorf("download source object: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create source file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write source file: %w", err)
	}

	return nil
}

func float64Ptr(v float64) *float64 {
	return &v
}
