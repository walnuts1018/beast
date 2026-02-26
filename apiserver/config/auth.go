package config

import "time"

type AuthMode string

const (
	AuthModeIntrospection AuthMode = "introspection"
	AuthModeStatic        AuthMode = "static"
)

type AuthConfig struct {
	Mode                  AuthMode      `env:"AUTH_MODE" envDefault:"introspection"`
	DevStaticToken        string        `env:"AUTH_DEV_STATIC_TOKEN"`
	DevStaticSubject      string        `env:"AUTH_DEV_STATIC_SUBJECT" envDefault:"dev-user"`
	IntrospectionCacheTTL time.Duration `env:"OIDC_INTROSPECTION_CACHE_TTL" envDefault:"30s"`
}
