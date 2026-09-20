package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	mediaKey   []byte
}

func NewS3Store(ctx context.Context, endpoint, region, bucket, accessKey, secretKey, stagingKey, mediaKey string) (*S3Store, error) {
	if endpoint == "" || region == "" || bucket == "" || accessKey == "" || secretKey == "" || stagingKey == "" || mediaKey == "" {
		return nil, fmt.Errorf("S3 endpoint, region, bucket, access key, secret key, staging encryption key, and media encryption key are required")
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
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return &S3Store{client: client, bucket: bucket, stagingKey: parsedStagingKey, mediaKey: parsedMediaKey}, nil
}

func (s *S3Store) PutEncrypted(ctx context.Context, objectKey string, src io.Reader) (crypto.AtRestResult, error) {
	temporary, err := os.CreateTemp("", "beast-encrypted-media-")
	if err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("create temporary encrypted object: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	result, encryptErr := crypto.EncryptToAtRest(temporary, src, s.mediaKey)
	closeErr := temporary.Close()
	if encryptErr != nil || closeErr != nil {
		return crypto.AtRestResult{}, errors.Join(encryptErr, closeErr)
	}
	file, err := os.Open(path)
	if err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("open encrypted temporary object: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: file, ContentType: aws.String("application/octet-stream")}); err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("put encrypted media object: %w", err)
	}
	return result, nil
}

func (s *S3Store) OpenDecrypted(ctx context.Context, objectKey string, metadata crypto.AtRestResult, start, end int64) (io.ReadCloser, error) {
	if start < 0 || end < start || end > metadata.PlaintextSize {
		return nil, fmt.Errorf("media range is invalid")
	}
	offset, length, _, err := crypto.EncryptedRange(metadata, start, end)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}
	objectRange := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Range: aws.String(objectRange)})
	if err != nil {
		return nil, fmt.Errorf("get encrypted media range: %w", err)
	}
	reader, writer := io.Pipe()
	go func() {
		decryptErr := crypto.DecryptRangeTo(writer, result.Body, metadata, s.mediaKey, start, end)
		closeErr := result.Body.Close()
		_ = writer.CloseWithError(errors.Join(decryptErr, closeErr))
	}()
	return &pipeReadCloser{PipeReader: reader, closeFn: func() error { _ = result.Body.Close(); return reader.Close() }}, nil
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
