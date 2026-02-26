package config

import (
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

type LogType string

const (
	LogTypeText LogType = "text"
	LogTypeJSON LogType = "json"
)

func ParseLogType(v string) (LogType, error) {
	switch v {
	case "text":
		return LogTypeText, nil
	case "json":
		return LogTypeJSON, nil
	default:
		return "", fmt.Errorf("invalid log type: %s", v)
	}
}

func ParseLogLevel(v string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(v)); err != nil {
		return 0, fmt.Errorf("invalid log level: %s", v)
	}
	return level, nil
}

type Config struct {
	LogLevel           slog.Level    `env:"LOG_LEVEL" envDefault:"info"`
	LogType            LogType       `env:"LOG_TYPE" envDefault:"json"`
	RabbitMQURL        string        `env:"RABBITMQ_URL" envDefault:"amqp://guest:guest@rabbitmq.rabbitmq.svc.cluster.local:5672/" validate:"required"`
	EncodeJobQueue     string        `env:"RABBITMQ_ENCODE_JOB_QUEUE" envDefault:"beast.encoder.jobs" validate:"required"`
	EncodeEventQueue   string        `env:"RABBITMQ_ENCODE_EVENT_QUEUE" envDefault:"beast.encoder.events" validate:"required"`
	ConsumerTag        string        `env:"RABBITMQ_CONSUMER_TAG" envDefault:"beast-encoder" validate:"required"`
	S3Region           string        `env:"S3_REGION" envDefault:"us-east-1" validate:"required"`
	S3Endpoint         string        `env:"S3_ENDPOINT" validate:"required"`
	S3Bucket           string        `env:"S3_BUCKET" validate:"required"`
	S3UsePathStyle     bool          `env:"S3_USE_PATH_STYLE" envDefault:"false"`
	PollInterval       time.Duration `env:"ENCODER_POLL_INTERVAL" envDefault:"5s"`
	WorkDir            string        `env:"ENCODER_WORK_DIR" envDefault:"/tmp/beast-encoder"`
	FFmpegPath         string        `env:"FFMPEG_PATH" envDefault:"ffmpeg"`
	FFprobePath        string        `env:"FFPROBE_PATH" envDefault:"ffprobe"`
	DashSegmentSeconds int           `env:"ENCODER_DASH_SEGMENT_SECONDS" envDefault:"4" validate:"gte=1,lte=30"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.ParseWithOptions(cfg, env.Options{
		FuncMap: map[reflect.Type]env.ParserFunc{
			reflect.TypeFor[slog.Level](): returnAny(ParseLogLevel),
			reflect.TypeFor[LogType]():    returnAny(ParseLogType),
		},
	}); err != nil {
		return nil, err
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func returnAny[T any](f func(v string) (t T, err error)) env.ParserFunc {
	return func(v string) (any, error) {
		t, err := f(v)
		return any(t), err
	}
}
