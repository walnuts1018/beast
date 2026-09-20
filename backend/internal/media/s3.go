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
	client *s3.Client
	bucket string
}

func NewS3Store(ctx context.Context, endpoint, region, bucket, accessKey, secretKey string) (*S3Store, error) {
	if endpoint == "" || region == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3 endpoint, region, bucket, access key, and secret key are required")
	}
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")))
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return &S3Store{client: client, bucket: bucket}, nil
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
