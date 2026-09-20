package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	RabbitMQURL           string
	JobQueue              string
	EventQueue            string
	ConsumerTag           string
	JobInput              string
	OutputRoot            string
	FFmpegPath            string
	FFprobePath           string
	SegmentSeconds        int
	ReencodeOnCopyFailure bool
	ShutdownTimeout       time.Duration
}

func Load() (Config, error) {
	segmentSeconds, err := intEnv("ENCODER_DASH_SEGMENT_SECONDS", 4)
	if err != nil {
		return Config{}, err
	}
	reencode, err := boolEnv("ENCODER_REENCODE_ON_COPY_FAILURE", true)
	if err != nil {
		return Config{}, err
	}
	return Config{
		RabbitMQURL:           envOr("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		JobQueue:              envOr("RABBITMQ_ENCODE_JOB_QUEUE", "beast.encoder.jobs"),
		EventQueue:            envOr("RABBITMQ_ENCODE_EVENT_QUEUE", "beast.encoder.events"),
		ConsumerTag:           envOr("RABBITMQ_CONSUMER_TAG", "beast-encoder"),
		JobInput:              os.Getenv("ENCODER_JOB_INPUT"),
		OutputRoot:            envOr("ENCODER_OUTPUT_ROOT", "/tmp/beast-encoder/output"),
		FFmpegPath:            envOr("FFMPEG_PATH", "ffmpeg"),
		FFprobePath:           envOr("FFPROBE_PATH", "ffprobe"),
		SegmentSeconds:        segmentSeconds,
		ReencodeOnCopyFailure: reencode,
		ShutdownTimeout:       10 * time.Second,
	}, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 30 {
		return 0, fmt.Errorf("%s must be an integer between 1 and 30", name)
	}
	return parsed, nil
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}
