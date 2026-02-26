package config

import (
	"fmt"
	"log/slog"
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
