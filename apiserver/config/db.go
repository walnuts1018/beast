package config

import "fmt"

type DBConfig struct {
	DatabaseURL string `env:"DATABASE_URL"`
	Host        string `env:"POSTGRES_HOST" envDefault:"postgresql-rw.postgres.svc.cluster.local" validate:"required"`
	Port        int    `env:"POSTGRES_PORT" envDefault:"5432" validate:"gte=1,lte=65535"`
	User        string `env:"POSTGRES_USER" envDefault:"beast" validate:"required"`
	Password    string `env:"POSTGRES_PASSWORD" envDefault:"password" validate:"required"`
	Name        string `env:"POSTGRES_DB" envDefault:"beast" validate:"required"`
	SSLMode     string `env:"POSTGRES_SSLMODE" envDefault:"disable" validate:"required"`
}

func (c DBConfig) DSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
		c.SSLMode,
	)
}
