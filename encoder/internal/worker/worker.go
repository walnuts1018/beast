package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Job struct {
	VideoID   string `json:"video_id"`
	InputPath string `json:"input_path"`
	OutputDir string `json:"output_dir"`
}

type Event struct {
	VideoID      string    `json:"video_id"`
	Status       string    `json:"status"`
	Progress     float64   `json:"progress"`
	ManifestPath string    `json:"manifest_path,omitempty"`
	Error        string    `json:"error,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type EventSink func(context.Context, Event) error

type Processor struct {
	Runner     *FFmpegRunner
	OutputRoot string
	Emit       EventSink
}

func (p *Processor) Process(ctx context.Context, job Job) error {
	if err := job.validate(); err != nil {
		return err
	}
	if p.Runner == nil || p.Emit == nil {
		return errors.New("worker dependencies are required")
	}
	if job.OutputDir == "" {
		job.OutputDir = filepath.Join(p.OutputRoot, job.VideoID)
	}
	if err := p.emit(ctx, Event{VideoID: job.VideoID, Status: "ENCODING"}); err != nil {
		return fmt.Errorf("emit encoding event: %w", err)
	}
	p.Runner.Progress = func(progress float64) {
		if err := p.emit(ctx, Event{VideoID: job.VideoID, Status: "ENCODING", Progress: progress}); err != nil {
			slog.WarnContext(ctx, "failed to emit encoding progress", "error", err)
		}
	}
	manifest, err := p.Runner.Process(ctx, job)
	if err != nil {
		publishErr := p.emit(ctx, Event{VideoID: job.VideoID, Status: "FAILED", Error: err.Error()})
		if publishErr != nil {
			return errors.Join(err, fmt.Errorf("emit failed event: %w", publishErr))
		}
		return err
	}
	if err := p.emit(ctx, Event{VideoID: job.VideoID, Status: "READY", Progress: 1, ManifestPath: manifest}); err != nil {
		return fmt.Errorf("emit ready event: %w", err)
	}
	return nil
}

func (p *Processor) emit(ctx context.Context, event Event) error {
	event.OccurredAt = time.Now().UTC()
	return p.Emit(ctx, event)
}

func (j Job) validate() error {
	if j.VideoID == "" {
		return errors.New("video_id is required")
	}
	if j.InputPath == "" {
		return errors.New("input_path is required")
	}
	return nil
}

type FFmpegRunner struct {
	FFmpegPath            string
	FFprobePath           string
	SegmentSeconds        int
	ReencodeOnCopyFailure bool
	Logger                *slog.Logger
	Progress              func(float64)
}

func (r *FFmpegRunner) Process(ctx context.Context, job Job) (string, error) {
	if r.FFmpegPath == "" || r.FFprobePath == "" {
		return "", errors.New("ffmpeg and ffprobe paths are required")
	}
	if r.SegmentSeconds < 1 || r.SegmentSeconds > 30 {
		return "", errors.New("segment seconds must be between 1 and 30")
	}
	if _, err := os.Stat(job.InputPath); err != nil {
		return "", fmt.Errorf("input media is unavailable: %w", err)
	}
	if err := os.MkdirAll(job.OutputDir, 0o750); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	duration, err := r.duration(ctx, job.InputPath)
	if err != nil {
		return "", err
	}
	manifestPath := filepath.Join(job.OutputDir, "manifest.mpd")
	if err := r.run(ctx, job.InputPath, manifestPath, duration, false); err != nil {
		if !r.ReencodeOnCopyFailure {
			return "", err
		}
		if fallbackErr := r.run(ctx, job.InputPath, manifestPath, duration, true); fallbackErr != nil {
			return "", errors.Join(err, fmt.Errorf("re-encode failed: %w", fallbackErr))
		}
	}
	return manifestPath, nil
}

func (r *FFmpegRunner) duration(ctx context.Context, input string) (float64, error) {
	cmd := exec.CommandContext(ctx, r.FFprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", input)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("probe media duration: %w", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) {
		return 0, errors.New("media duration is invalid")
	}
	return duration, nil
}

func (r *FFmpegRunner) run(ctx context.Context, input, manifest string, duration float64, reencode bool) error {
	args := dashArgs(input, manifest, r.SegmentSeconds, reencode)
	cmd := exec.CommandContext(ctx, r.FFmpegPath, args...)
	progress := &progressReader{duration: duration, callback: r.Progress}
	cmd.Stdout = progress
	cmd.Stderr = stderrWriter{logger: r.Logger}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg DASH conversion failed: %w", err)
	}
	if err := progress.Err(); err != nil {
		return err
	}
	return nil
}

func dashArgs(input, manifest string, segmentSeconds int, reencode bool) []string {
	args := []string{"-hide_banner", "-nostats", "-y", "-progress", "pipe:1", "-i", input, "-map", "0:v:0", "-map", "0:a?"}
	if reencode {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-c:a", "aac", "-b:a", "128k")
	} else {
		args = append(args, "-c", "copy")
	}
	return append(args, "-f", "dash", "-seg_duration", strconv.Itoa(segmentSeconds), "-use_template", "1", "-use_timeline", "1", manifest)
}

type progressReader struct {
	duration float64
	buffer   bytes.Buffer
	progress float64
	callback func(float64)
}

func (r *progressReader) Write(p []byte) (int, error) {
	_, _ = r.buffer.Write(p)
	for {
		line, err := r.buffer.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return 0, err
			}
			break
		}
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && key == "out_time_ms" {
			microseconds, parseErr := strconv.ParseFloat(value, 64)
			if parseErr == nil && r.duration > 0 {
				r.progress = min(0.99, max(0, microseconds/1_000_000/r.duration))
				if r.callback != nil {
					r.callback(r.progress)
				}
			}
		}
	}
	return len(p), nil
}

func (r *progressReader) Err() error { return nil }

type stderrWriter struct{ logger *slog.Logger }

func (w stderrWriter) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line != "" && w.logger != nil {
		w.logger.Debug("ffmpeg output", "line", line)
	}
	return len(p), nil
}
