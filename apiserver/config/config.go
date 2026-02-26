package config

import (
	"fmt"
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
	Auth     AuthConfig
	OIDC     OIDCConfig
	DB       DBConfig
	S3       S3Config
	RabbitMQ RabbitMQConfig
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

	if err := validateAuthRelatedConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateAuthRelatedConfig(cfg *Config) error {
	switch cfg.Auth.Mode {
	case AuthModeIntrospection:
		if cfg.OIDC.IntrospectionURL == "" {
			return fmt.Errorf("OIDC_INTROSPECTION_URL is required when AUTH_MODE=introspection")
		}
		if cfg.OIDC.ClientID == "" {
			return fmt.Errorf("OIDC_CLIENT_ID is required when AUTH_MODE=introspection")
		}
		if cfg.OIDC.ClientSecret == "" {
			return fmt.Errorf("OIDC_CLIENT_SECRET is required when AUTH_MODE=introspection")
		}
	case AuthModeStatic:
		if cfg.Auth.DevStaticToken == "" {
			return fmt.Errorf("AUTH_DEV_STATIC_TOKEN is required when AUTH_MODE=static")
		}
	default:
		return fmt.Errorf("unsupported AUTH_MODE: %s", cfg.Auth.Mode)
	}

	return nil
}

func returnAny[T any](f func(v string) (t T, err error)) env.ParserFunc {
	return func(v string) (any, error) {
		t, err := f(v)
		return any(t), err
	}
}
