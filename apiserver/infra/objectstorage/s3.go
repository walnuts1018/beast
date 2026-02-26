package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type S3Storage struct {
	bucket    string
	presigner *s3.PresignClient
	client    *s3.Client
	uploadTTL time.Duration
}

type Config struct {
	Region          string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	UploadTTL       time.Duration
}

func NewS3Storage(ctx context.Context, cfg Config) (*S3Storage, error) {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.UploadTTL <= 0 {
		cfg.UploadTTL = 15 * time.Minute
	}

	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}

	loadOptions := make([]func(*config.LoadOptions) error, 0, 3)
	loadOptions = append(loadOptions, config.WithRegion(cfg.Region))

	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		loadOptions = append(loadOptions, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	if cfg.Endpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
			if service == s3.ServiceID {
				return aws.Endpoint{
					URL:               cfg.Endpoint,
					HostnameImmutable: true,
				}, nil
			}

			return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested: %s (%s)", service, region)
		})
		loadOptions = append(loadOptions, config.WithEndpointResolverWithOptions(resolver))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.UsePathStyle
	})

	storage := &S3Storage{
		bucket:    cfg.Bucket,
		presigner: s3.NewPresignClient(client),
		client:    client,
		uploadTTL: cfg.UploadTTL,
	}

	if err := storage.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *S3Storage) CreateUploadURL(ctx context.Context, objectKey string, contentType string, expiresIn time.Duration) (string, error) {
	ttl := expiresIn
	if ttl <= 0 {
		ttl = s.uploadTTL
	}

	request, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign put object: %w", err)
	}

	return request.URL, nil
}

func (s *S3Storage) Exists(ctx context.Context, objectKey string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err == nil {
		return true, nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotFound" {
		return false, nil
	}

	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return false, nil
	}

	return false, fmt.Errorf("head object: %w", err)
}

func (s *S3Storage) HealthCheck(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		return fmt.Errorf("head bucket %s: %w", s.bucket, err)
	}

	return nil
}

func (s *S3Storage) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchBucket":
			_, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
			if createErr != nil {
				return fmt.Errorf("create bucket %s: %w", s.bucket, createErr)
			}
			return nil
		}
	}

	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		_, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
		if createErr != nil {
			return fmt.Errorf("create bucket %s: %w", s.bucket, createErr)
		}
		return nil
	}

	return fmt.Errorf("head bucket %s: %w", s.bucket, err)
}
