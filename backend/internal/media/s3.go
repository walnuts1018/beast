package media

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/walnuts1018/beast/backend/internal/crypto"
)

type S3Store struct {
	client     *s3.Client
	bucket     string
	stagingKey []byte
}

func NewS3Store(ctx context.Context, endpoint, region, bucket, accessKey, secretKey, stagingKey string) (*S3Store, error) {
	if endpoint == "" || region == "" || bucket == "" || accessKey == "" || secretKey == "" || stagingKey == "" {
		return nil, fmt.Errorf("S3 endpoint, region, bucket, access key, secret key, and staging encryption key are required")
	}
	parsedStagingKey, err := crypto.ParseStagingKey(stagingKey)
	if err != nil {
		return nil, err
	}
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")))
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return &S3Store{client: client, bucket: bucket, stagingKey: parsedStagingKey}, nil
}

func (s *S3Store) Save(ctx context.Context, objectKey string, src io.Reader, publicKey string) (crypto.Result, error) {
	temporary, err := os.CreateTemp("", "beast-encrypted-*")
	if err != nil {
		return crypto.Result{}, fmt.Errorf("create temporary encrypted object: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	result, err := crypto.EncryptTo(temporary, src, publicKey)
	if closeErr := temporary.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return crypto.Result{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return crypto.Result{}, fmt.Errorf("open encrypted temporary object: %w", err)
	}
	defer func() { _ = file.Close() }()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: file, ContentType: aws.String("application/octet-stream")})
	if err != nil {
		return crypto.Result{}, fmt.Errorf("put encrypted object: %w", err)
	}
	return result, nil
}

func (s *S3Store) Open(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		return nil, fmt.Errorf("get encrypted object: %w", err)
	}
	return result.Body, nil
}

func (s *S3Store) Check(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		return fmt.Errorf("check S3 bucket: %w", err)
	}
	return nil
}

var _ ObjectStore = (*S3Store)(nil)
var _ SourceStore = (*S3Store)(nil)

func (s *S3Store) SaveSource(ctx context.Context, objectKey string, src io.Reader) error {
	temporary, err := os.CreateTemp("", "beast-staging-")
	if err != nil {
		return fmt.Errorf("create temporary staging object: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	if err := crypto.EncryptStagingTo(temporary, src, s.stagingKey); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary staging object: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open temporary staging object: %w", err)
	}
	defer func() { _ = file.Close() }()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: file, ContentType: aws.String("application/octet-stream")})
	if err != nil {
		return fmt.Errorf("put source object: %w", err)
	}
	return nil
}

func (s *S3Store) Delete(ctx context.Context, objectKey string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		return fmt.Errorf("delete source object: %w", err)
	}
	return nil
}
