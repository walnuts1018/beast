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
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/walnuts1018/beast/encoder/internal/rabbitmq"
)

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

type Worker struct {
	rmq                *rabbitmq.Client
	s3Client           *s3.Client
	bucket             string
	workDir            string
	ffmpegPath         string
	ffprobePath        string
	dashSegmentSeconds int
	jobTimeout         time.Duration
	logger             *slog.Logger
	runner             commandRunner

	encoderProbeOnce sync.Once
	encoderProbeErr  error
	encoders         map[string]struct{}
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

type ffmpegPlan struct {
	copyVideo    bool
	copyAudio    bool
	videoEncoder string
	audioEncoder string
}

var dashCopyVideoCodecs = map[string]struct{}{
	"h264":   {},
	"hevc":   {},
	"h265":   {},
	"av1":    {},
	"vp9":    {},
	"vp8":    {},
	"mpeg4":  {},
	"theora": {},
}

var dashCopyAudioCodecs = map[string]struct{}{
	"aac":    {},
	"mp3":    {},
	"opus":   {},
	"vorbis": {},
	"ac3":    {},
	"eac3":   {},
	"flac":   {},
}

func New(
	rmq *rabbitmq.Client,
	s3Client *s3.Client,
	bucket string,
	workDir string,
	ffmpegPath string,
	ffprobePath string,
	dashSegmentSeconds int,
	jobTimeout time.Duration,
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
	if jobTimeout <= 0 {
		jobTimeout = 6 * time.Hour
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
		jobTimeout:         jobTimeout,
		logger:             logger,
		runner:             execCommandRunner{},
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

			if ctx.Err() != nil {
				// シャットダウン時はジョブを再キューイングするだけにする。
				// 失敗イベントは送信しない（再キューされたジョブが再処理時にFAILEDになることを防ぐ）。
				_ = delivery.Nack(false, true)
				w.logger.InfoContext(ctx, "ジョブを再キューイングしてシャットダウン", slog.String("videoID", job.VideoID))
				return nil
			}

			// エンコード処理はキャンセルされない独立したコンテキストで実行する。
			// これにより、シャットダウンシグナルを受けても処理中のジョブは完了する。
			// ただし無限にブロックしないよう、タイムアウトを設定する。
			jobCtx, jobCancel := context.WithTimeout(context.WithoutCancel(ctx), w.jobTimeout)
			if err := w.encodeDash(jobCtx, job); err != nil {
				jobCancel()
				reason := err.Error()
				// エンコード失敗はジョブを永続的な失敗として扱い、Ackする。
				// これは無限リトライループを回避するための設計判断である。
				// 一時的な障害（S3接続エラーなど）の場合は、APIサーバー側の
				// RetryEncoding機能を通じて手動リトライが可能。
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
			jobCancel()
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
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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

func (w *Worker) runFFmpegDash(ctx context.Context, inputPath string, manifestPath string, meta mediaMeta) error {
	encoders, encoderErr := w.getAvailableEncoders(ctx)
	if encoderErr != nil {
		w.logger.WarnContext(ctx, "failed to probe ffmpeg encoders; fallback to software defaults", slog.Any("error", encoderErr))
		encoders = nil
	}

	initialPlan := w.buildInitialPlan(meta, encoders)
	initialArgs := w.buildFFmpegDashArgs(inputPath, manifestPath, initialPlan)
	output, err := w.runner.Run(ctx, w.ffmpegPath, initialArgs...)
	if err == nil {
		return nil
	}

	if !initialPlan.copyVideo && !initialPlan.copyAudio {
		return fmt.Errorf("ffmpeg dash failed: %w: %s", err, strings.TrimSpace(output))
	}

	if encoderErr != nil {
		return fmt.Errorf("ffmpeg dash failed: %w: %s", err, strings.TrimSpace(output))
	}

	fallbackPlan := w.buildFallbackPlan(encoders)
	fallbackArgs := w.buildFFmpegDashArgs(inputPath, manifestPath, fallbackPlan)
	fallbackOutput, fallbackErr := w.runner.Run(ctx, w.ffmpegPath, fallbackArgs...)
	if fallbackErr != nil {
		return fmt.Errorf("ffmpeg dash failed (initial: %s, fallback: %s)", strings.TrimSpace(output), strings.TrimSpace(fallbackOutput))
	}

	w.logger.WarnContext(ctx, "ffmpeg initial plan failed, fallback reencode succeeded", slog.String("videoCodec", meta.VideoCodec), slog.String("audioCodec", meta.AudioCodec), slog.String("fallbackVideoEncoder", fallbackPlan.videoEncoder))
	return nil
}

func (w *Worker) buildInitialPlan(meta mediaMeta, encoders map[string]struct{}) ffmpegPlan {
	videoCodec := strings.ToLower(strings.TrimSpace(meta.VideoCodec))
	audioCodec := strings.ToLower(strings.TrimSpace(meta.AudioCodec))
	_, canCopyVideo := dashCopyVideoCodecs[videoCodec]
	_, canCopyAudio := dashCopyAudioCodecs[audioCodec]
	if audioCodec == "" {
		canCopyAudio = true
	}

	return ffmpegPlan{
		copyVideo:    canCopyVideo,
		copyAudio:    canCopyAudio,
		videoEncoder: chooseBestVideoEncoder(encoders),
		audioEncoder: chooseBestAudioEncoder(encoders),
	}
}

func (w *Worker) buildFallbackPlan(encoders map[string]struct{}) ffmpegPlan {
	return ffmpegPlan{
		copyVideo:    false,
		copyAudio:    false,
		videoEncoder: chooseBestVideoEncoder(encoders),
		audioEncoder: chooseBestAudioEncoder(encoders),
	}
}

func (w *Worker) buildFFmpegDashArgs(inputPath string, manifestPath string, plan ffmpegPlan) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a?",
	}

	if plan.copyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", plan.videoEncoder)
		args = append(args, videoEncoderOptions(plan.videoEncoder)...)
	}

	if plan.copyAudio {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", plan.audioEncoder)
		args = append(args, audioEncoderOptions(plan.audioEncoder)...)
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

	return args
}

func chooseBestVideoEncoder(encoders map[string]struct{}) string {
	preferred := []string{
		"h264_nvenc",
		"h264_qsv",
		"h264_videotoolbox",
		"av1_nvenc",
		"av1_qsv",
		"av1_videotoolbox",
		"libx264",
		"libsvtav1",
	}
	for _, encoder := range preferred {
		if _, ok := encoders[encoder]; ok {
			return encoder
		}
	}
	return "libx264"
}

func chooseBestAudioEncoder(encoders map[string]struct{}) string {
	preferred := []string{"aac", "libopus"}
	for _, encoder := range preferred {
		if _, ok := encoders[encoder]; ok {
			return encoder
		}
	}
	return "aac"
}

func videoEncoderOptions(encoder string) []string {
	switch encoder {
	case "libx264":
		return []string{"-preset", "veryfast", "-crf", "22"}
	case "libsvtav1":
		return []string{"-preset", "8", "-crf", "32"}
	case "h264_nvenc", "av1_nvenc":
		return []string{"-preset", "p4", "-cq", "28", "-b:v", "0"}
	case "h264_qsv", "av1_qsv":
		return []string{"-global_quality", "26"}
	case "h264_videotoolbox", "av1_videotoolbox":
		return []string{"-b:v", "0", "-q:v", "65"}
	default:
		return nil
	}
}

func audioEncoderOptions(encoder string) []string {
	switch encoder {
	case "aac":
		return []string{"-b:a", "128k"}
	case "libopus":
		return []string{"-b:a", "96k"}
	default:
		return nil
	}
}

func (w *Worker) getAvailableEncoders(ctx context.Context) (map[string]struct{}, error) {
	w.encoderProbeOnce.Do(func() {
		output, err := w.runner.Run(ctx, w.ffmpegPath, "-hide_banner", "-encoders")
		if err != nil {
			w.encoderProbeErr = fmt.Errorf("probe ffmpeg encoders: %w", err)
			return
		}

		result := make(map[string]struct{})
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "------") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			name := fields[1]
			result[name] = struct{}{}
		}
		w.encoders = result
	})

	if w.encoderProbeErr != nil {
		return nil, w.encoderProbeErr
	}
	return w.encoders, nil
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

func (w *Worker) probeInput(ctx context.Context, inputPath string) (mediaMeta, error) {
	output, err := w.runner.Run(
		ctx,
		w.ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		inputPath,
	)
	if err != nil {
		return mediaMeta{}, fmt.Errorf("ffprobe failed: %w", err)
	}

	var result probeResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
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
		defer func() {
			_ = f.Close()
		}()

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
	defer func() {
		_ = resp.Body.Close()
	}()

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create source file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		// io.Copy失敗時は明示的にファイルをクローズしてから不完全なファイルを削除する
		_ = f.Close()
		if removeErr := os.Remove(outputPath); removeErr != nil {
			w.logger.Warn("不完全なソースファイルの削除に失敗", slog.Any("error", removeErr), slog.String("path", outputPath))
		}
		return fmt.Errorf("write source file: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close source file: %w", err)
	}

	return nil
}

func float64Ptr(v float64) *float64 {
	return &v
}
