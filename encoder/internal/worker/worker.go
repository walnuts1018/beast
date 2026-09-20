package worker

import (
	"context"
	"encoding/json"
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

	"github.com/walnuts1018/beast/encoder/internal/crypto"
)

const ContractVersion = "v2"

type Job struct {
	ContractVersion string `json:"contract_version"`
	VideoID         string `json:"video_id"`
	OwnerID         string `json:"owner_id"`
	SourceObjectKey string `json:"source_object_key"`
	OutputPrefix    string `json:"output_prefix"`
}

type EncryptionMetadata struct {
	Algorithm        string `json:"algorithm"`
	ChunkSize        int    `json:"chunk_size"`
	Nonce            string `json:"nonce"`
	EncryptedDataKey string `json:"encrypted_data_key"`
	PlaintextSize    int64  `json:"plaintext_size"`
}

type Artifact struct {
	ObjectKey  string             `json:"object_key"`
	Encryption EncryptionMetadata `json:"encryption"`
}

type Event struct {
	ContractVersion string              `json:"contract_version"`
	VideoID         string              `json:"video_id"`
	OwnerID         string              `json:"owner_id"`
	SourceObjectKey string              `json:"source_object_key"`
	Status          string              `json:"status"`
	Progress        float64             `json:"progress"`
	Manifest        Artifact            `json:"manifest"`
	Artifacts       map[string]Artifact `json:"artifacts"`
	Error           string              `json:"error,omitempty"`
	OccurredAt      time.Time           `json:"occurred_at"`
}

type EventSink func(context.Context, Event) error

type ArtifactStore interface {
	Open(context.Context, string) (io.ReadCloser, error)
	PutEncrypted(context.Context, string, io.Reader) (crypto.AtRestResult, error)
	Delete(context.Context, string) error
}

type Processor struct {
	Runner     *FFmpegRunner
	Store      ArtifactStore
	OutputRoot string
	Emit       EventSink
}

func (p *Processor) Process(ctx context.Context, job Job) error {
	if err := job.validate(); err != nil {
		return err
	}
	if p.Runner == nil || p.Store == nil || p.Emit == nil {
		return errors.New("worker dependencies are required")
	}
	if p.OutputRoot == "" {
		return errors.New("output root is required")
	}
	workDir, err := os.MkdirTemp(p.OutputRoot, "job-")
	if err != nil {
		return fmt.Errorf("create encoder work directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	source, err := p.Store.Open(ctx, job.SourceObjectKey)
	if err != nil {
		return fmt.Errorf("open staged source: %w", err)
	}
	inputPath := filepath.Join(workDir, "input")
	input, err := os.OpenFile(inputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("create encoder input: %w", err)
	}
	_, copyErr := io.Copy(input, source)
	closeInputErr := input.Close()
	closeSourceErr := source.Close()
	if copyErr != nil || closeInputErr != nil || closeSourceErr != nil {
		deleteErr := p.Store.Delete(ctx, job.SourceObjectKey)
		return errors.Join(copyErr, closeInputErr, closeSourceErr, deleteErr)
	}
	if err := p.emit(ctx, job, Event{Status: "ENCODING", Progress: 0}); err != nil {
		return fmt.Errorf("emit encoding event: %w", err)
	}
	p.Runner.Progress = func(progress float64) {
		if err := p.emit(ctx, job, Event{Status: "ENCODING", Progress: progress}); err != nil {
			slog.WarnContext(ctx, "failed to emit encoding progress", "error", err)
		}
	}
	outputDir := filepath.Join(workDir, "output")
	if _, err := p.Runner.Process(ctx, inputPath, outputDir); err != nil {
		publishErr := p.emit(ctx, job, Event{Status: "FAILED", Error: err.Error()})
		if publishErr != nil {
			return errors.Join(err, fmt.Errorf("emit failed event: %w", publishErr))
		}
		if deleteErr := p.Store.Delete(ctx, job.SourceObjectKey); deleteErr != nil {
			return fmt.Errorf("delete staged source after failure: %w", deleteErr)
		}
		return nil
	}
	artifacts, manifest, err := p.encryptArtifacts(ctx, job, outputDir)
	if err != nil {
		publishErr := p.emit(ctx, job, Event{Status: "FAILED", Error: err.Error()})
		if publishErr != nil {
			return errors.Join(err, fmt.Errorf("emit failed event: %w", publishErr))
		}
		if deleteErr := p.Store.Delete(ctx, job.SourceObjectKey); deleteErr != nil {
			return fmt.Errorf("delete staged source after failure: %w", deleteErr)
		}
		return nil
	}
	if err := p.emit(ctx, job, Event{Status: "READY", Progress: 1, Manifest: manifest, Artifacts: artifacts}); err != nil {
		return fmt.Errorf("emit ready event: %w", err)
	}
	if err := p.Store.Delete(ctx, job.SourceObjectKey); err != nil {
		return fmt.Errorf("delete staged source: %w", err)
	}
	return nil
}

func (p *Processor) encryptArtifacts(ctx context.Context, job Job, outputDir string) (map[string]Artifact, Artifact, error) {
	artifacts := make(map[string]Artifact)
	var manifest Artifact
	err := filepath.WalkDir(outputDir, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(outputDir, filePath)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		objectKey := strings.TrimSuffix(job.OutputPrefix, "/") + "/" + name
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		result, encryptErr := p.Store.PutEncrypted(ctx, objectKey, file)
		closeErr := file.Close()
		if encryptErr != nil || closeErr != nil {
			return errors.Join(encryptErr, closeErr)
		}
		artifact := Artifact{ObjectKey: objectKey, Encryption: EncryptionMetadata{Algorithm: result.Algorithm, ChunkSize: result.ChunkSize, Nonce: result.Nonce, EncryptedDataKey: result.EncryptedDataKey, PlaintextSize: result.PlaintextSize}}
		artifacts[name] = artifact
		if name == "manifest.m3u8" {
			manifest = artifact
		}
		return nil
	})
	if err != nil {
		return nil, Artifact{}, fmt.Errorf("encrypt HLS artifacts: %w", err)
	}
	if manifest.ObjectKey == "" {
		return nil, Artifact{}, errors.New("HLS manifest was not produced")
	}
	return artifacts, manifest, nil
}

func (p *Processor) emit(ctx context.Context, job Job, event Event) error {
	event.ContractVersion = ContractVersion
	event.VideoID = job.VideoID
	event.OwnerID = job.OwnerID
	event.SourceObjectKey = job.SourceObjectKey
	event.OccurredAt = time.Now().UTC()
	return p.Emit(ctx, event)
}

func (j Job) validate() error {
	if j.ContractVersion != ContractVersion || j.VideoID == "" || j.OwnerID == "" || j.SourceObjectKey == "" || j.OutputPrefix == "" {
		return errors.New("encoder job contract is incomplete")
	}
	if strings.Contains(j.OutputPrefix, "..") || strings.HasPrefix(j.SourceObjectKey, "/") || strings.Contains(j.SourceObjectKey, "..") {
		return errors.New("encoder job object keys are invalid")
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

func (r *FFmpegRunner) Process(ctx context.Context, inputPath, outputDir string) (string, error) {
	if r.FFmpegPath == "" || r.FFprobePath == "" {
		return "", errors.New("ffmpeg and ffprobe paths are required")
	}
	if r.SegmentSeconds < 2 || r.SegmentSeconds > 4 {
		return "", errors.New("segment seconds must be between 2 and 4")
	}
	if _, err := os.Stat(inputPath); err != nil {
		return "", fmt.Errorf("input media is unavailable: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	duration, err := r.duration(ctx, inputPath)
	if err != nil {
		return "", err
	}
	manifestPath := filepath.Join(outputDir, "manifest.m3u8")
	copyCompatible, err := r.copyCompatible(ctx, inputPath)
	if err != nil {
		return "", err
	}
	if !copyCompatible {
		if err := r.run(ctx, inputPath, manifestPath, duration, true); err != nil {
			return "", err
		}
		return manifestPath, nil
	}
	if err := r.run(ctx, inputPath, manifestPath, duration, false); err != nil {
		if !r.ReencodeOnCopyFailure {
			return "", err
		}
		if fallbackErr := r.run(ctx, inputPath, manifestPath, duration, true); fallbackErr != nil {
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

func (r *FFmpegRunner) copyCompatible(ctx context.Context, input string) (bool, error) {
	cmd := exec.CommandContext(ctx, r.FFprobePath, "-v", "error", "-show_entries", "stream=codec_type,codec_name", "-of", "json", input)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("probe media codecs: %w", err)
	}
	compatible, err := codecsAllowStreamCopy(output)
	if err != nil {
		return false, err
	}
	return compatible, nil
}

func codecsAllowStreamCopy(output []byte) (bool, error) {
	var result struct {
		Streams []struct {
			Type  string `json:"codec_type"`
			Codec string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, fmt.Errorf("decode media codecs: %w", err)
	}
	videoCount, audioCount := 0, 0
	for _, stream := range result.Streams {
		switch stream.Type {
		case "video":
			videoCount++
			if videoCount > 1 || (stream.Codec != "h264" && stream.Codec != "hevc" && stream.Codec != "av1") {
				return false, nil
			}
		case "audio":
			audioCount++
			if audioCount > 1 || stream.Codec != "aac" {
				return false, nil
			}
		}
	}
	return videoCount == 1, nil
}

func (r *FFmpegRunner) run(ctx context.Context, input, manifest string, duration float64, reencode bool) error {
	args := hlsArgs(input, manifest, r.SegmentSeconds, reencode)
	cmd := exec.CommandContext(ctx, r.FFmpegPath, args...)
	progress := &progressReader{duration: duration, callback: r.Progress}
	cmd.Stdout = progress
	cmd.Stderr = stderrWriter{logger: r.Logger}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg HLS conversion failed: %w", err)
	}
	return progress.Err()
}

func hlsArgs(input, manifest string, segmentSeconds int, reencode bool) []string {
	args := []string{"-hide_banner", "-nostats", "-y", "-progress", "pipe:1", "-i", input, "-map", "0:v:0", "-map", "0:a:0?"}
	if reencode {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segmentSeconds), "-c:a", "aac", "-b:a", "128k")
	} else {
		args = append(args, "-c", "copy")
	}
	segmentPattern := filepath.Join(filepath.Dir(manifest), "segment_%05d.m4s")
	return append(args, "-f", "hls", "-hls_segment_type", "fmp4", "-hls_time", strconv.Itoa(segmentSeconds), "-hls_playlist_type", "vod", "-hls_flags", "independent_segments", "-hls_fmp4_init_filename", "init.mp4", "-hls_segment_filename", segmentPattern, manifest)
}

type progressReader struct {
	duration float64
	buffer   []byte
	callback func(float64)
}

func (r *progressReader) Write(p []byte) (int, error) {
	r.buffer = append(r.buffer, p...)
	for {
		index := strings.IndexByte(string(r.buffer), '\n')
		if index < 0 {
			break
		}
		line := string(r.buffer[:index])
		r.buffer = r.buffer[index+1:]
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && key == "out_time_ms" {
			microseconds, parseErr := strconv.ParseFloat(value, 64)
			if parseErr == nil && r.duration > 0 && r.callback != nil {
				r.callback(min(0.99, max(0, microseconds/1_000_000/r.duration)))
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
