package logger

import (
	"log/slog"
	"os"

	"github.com/walnuts1018/beast/encoder/internal/config"
)

func New(logLevel slog.Level, logType config.LogType) *slog.Logger {
	handlerOptions := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	var handler slog.Handler
	switch logType {
	case config.LogTypeText:
		handler = slog.NewTextHandler(os.Stdout, handlerOptions)
	case config.LogTypeJSON:
		fallthrough
	default:
		handler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	}

	return slog.New(handler)
}
