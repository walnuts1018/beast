package config

import "time"

type S3Config struct {
	Bucket       string        `env:"S3_BUCKET,required"`
	UsePathStyle bool          `env:"S3_USE_PATH_STYLE" envDefault:"false"`
	UploadURLTTL time.Duration `env:"UPLOAD_URL_TTL" envDefault:"15m"`
}
