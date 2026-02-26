package config

type ServerConfig struct {
	Port int `env:"PORT" envDefault:"8080" validate:"gte=1,lte=65535"`
}
