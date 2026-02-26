package config

type OIDCConfig struct {
	IntrospectionURL string `env:"OIDC_INTROSPECTION_URL" validate:"required"`
	ClientID         string `env:"OIDC_CLIENT_ID" validate:"required"`
	ClientSecret     string `env:"OIDC_CLIENT_SECRET" validate:"required"`
}
