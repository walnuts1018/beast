package config

import "time"

type S3Config struct {
	Region          string        `env:"S3_REGION" envDefault:"us-east-1" validate:"required"`
	Endpoint        string        `env:"S3_ENDPOINT" validate:"required"`
	Bucket          string        `env:"S3_BUCKET" envDefault:"beast-upload-temp" validate:"required"`
	AccessKeyID     string        `env:"S3_ACCESS_KEY_ID" validate:"required"`
	SecretAccessKey string        `env:"S3_SECRET_ACCESS_KEY" validate:"required"`
	UsePathStyle    bool          `env:"S3_USE_PATH_STYLE" envDefault:"true"`
	UploadURLTTL    time.Duration `env:"UPLOAD_URL_TTL" envDefault:"15m"`
}
