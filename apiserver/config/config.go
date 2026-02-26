package config

import (
	"log/slog"
	"reflect"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	Server   ServerConfig
	LogLevel slog.Level `env:"LOG_LEVEL" envDefault:"info"`
	LogType  LogType    `env:"LOG_TYPE" envDefault:"json"`
	OIDC     OIDCConfig
	DB       DBConfig
	S3       S3Config
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.ParseWithOptions(cfg, env.Options{
		FuncMap: map[reflect.Type]env.ParserFunc{
			reflect.TypeOf(slog.Level(0)): returnAny(ParseLogLevel),
			reflect.TypeOf(LogType("")):   returnAny(ParseLogType),
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
