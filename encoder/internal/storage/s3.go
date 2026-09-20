package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/walnuts1018/beast/encoder/internal/crypto"
)

type S3 struct {
	client     *s3.Client
	bucket     string
	stagingKey []byte
	mediaKey   []byte
}

func NewS3(ctx context.Context, endpoint, region, bucket, accessKey, secretKey, stagingKey, mediaKey string) (*S3, error) {
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
	parsedMediaKey, err := crypto.ParseMasterKey(mediaKey)
	if err != nil {
		return nil, err
	}
	return &S3{client: s3.NewFromConfig(awsConfig, func(options *s3.Options) { options.BaseEndpoint = aws.String(endpoint); options.UsePathStyle = true }), bucket: bucket, stagingKey: parsedStagingKey, mediaKey: parsedMediaKey}, nil
}

func (s *S3) Check(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("check S3 bucket: %w", err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		return nil, fmt.Errorf("get S3 source object: %w", err)
	}
	temporary, err := os.CreateTemp("", "beast-encoder-source-")
	if err != nil {
		_ = result.Body.Close()
		return nil, fmt.Errorf("create decrypted source: %w", err)
	}
	path := temporary.Name()
	if err := crypto.DecryptStagingTo(temporary, result.Body, s.stagingKey); err != nil {
		_ = result.Body.Close()
		_ = temporary.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := result.Body.Close(); err != nil {
		_ = temporary.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("close staged source: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close decrypted source: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("open decrypted source: %w", err)
	}
	return &removingReadCloser{File: file, path: path}, nil
}

type removingReadCloser struct {
	*os.File
	path string
}

func (r *removingReadCloser) Close() error {
	closeErr := r.File.Close()
	removeErr := os.Remove(r.path)
	if closeErr != nil || (removeErr != nil && !os.IsNotExist(removeErr)) {
		return errors.Join(closeErr, removeErr)
	}
	return nil
}

func (s *S3) PutEncrypted(ctx context.Context, objectKey string, source io.Reader) (crypto.AtRestResult, error) {
	temporary, err := os.CreateTemp("", "beast-encoder-encrypted-")
	if err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("create temporary encrypted object: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	result, encryptErr := crypto.EncryptToAtRest(temporary, source, s.mediaKey)
	closeErr := temporary.Close()
	if encryptErr != nil || closeErr != nil {
		return crypto.AtRestResult{}, fmt.Errorf("encrypt output artifact: %w", errors.Join(encryptErr, closeErr))
	}
	file, err := os.Open(path)
	if err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("open encrypted output artifact: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: file, ContentType: aws.String("application/octet-stream")}); err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("put encrypted output artifact: %w", err)
	}
	return result, nil
}

func (s *S3) Delete(ctx context.Context, objectKey string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)}); err != nil {
		return fmt.Errorf("delete staged source object: %w", err)
	}
	return nil
}
