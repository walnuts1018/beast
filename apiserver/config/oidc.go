package config

type OIDCConfig struct {
	IntrospectionURL string `env:"OIDC_INTROSPECTION_URL"`
	ClientID         string `env:"OIDC_CLIENT_ID"`
	ClientSecret     string `env:"OIDC_CLIENT_SECRET"`
}
