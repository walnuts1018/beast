package logger

import (
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
	slogctx "github.com/veqryn/slog-context"
	"github.com/walnuts1018/beast/apiserver/config"
)

func CreateLogger(logLevel slog.Level, logType config.LogType) *slog.Logger {
	var handler slog.Handler
	switch logType {
	case config.LogTypeText:
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:     logLevel,
			AddSource: logLevel == slog.LevelDebug,
		})
	case config.LogTypeJSON:
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     logLevel,
			AddSource: logLevel == slog.LevelDebug,
		})
	}

	return slog.New(slogctx.NewHandler(handler, nil))
}
