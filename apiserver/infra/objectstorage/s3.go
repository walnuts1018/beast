package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/walnuts1018/beast/apiserver/config"
)

type S3Storage struct {
	bucket    string
	presigner *s3.PresignClient
	client    *s3.Client
	uploadTTL time.Duration
}

func NewS3Storage(ctx context.Context, awsCfg aws.Config, cfg config.S3Config) (*S3Storage, error) {
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
	})

	storage := &S3Storage{
		bucket:    cfg.Bucket,
		presigner: s3.NewPresignClient(client),
		client:    client,
		uploadTTL: cfg.UploadURLTTL,
	}

	if err := storage.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *S3Storage) CreateUploadURL(ctx context.Context, objectKey string, expiresIn time.Duration) (string, error) {
	ttl := expiresIn
	if ttl <= 0 {
		ttl = s.uploadTTL
	}

	request, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
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

func (s *S3Storage) Download(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}

	return output.Body, nil
}

func (s *S3Storage) Upload(ctx context.Context, objectKey string, body io.Reader, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	return nil
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
