package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/walnuts1018/beast/backend/internal/crypto"
)

type LocalStore struct {
	root       string
	stagingKey []byte
	mediaKey   []byte
}

type ObjectStore interface {
	PutEncrypted(context.Context, string, io.Reader) (crypto.AtRestResult, error)
	OpenDecrypted(context.Context, string, crypto.AtRestResult, int64, int64) (io.ReadCloser, error)
}

type pipeReadCloser struct {
	*io.PipeReader
	closeFn func() error
}

func (r *pipeReadCloser) Close() error {
	pipeErr := r.PipeReader.Close()
	if r.closeFn == nil {
		return pipeErr
	}
	return errors.Join(pipeErr, r.closeFn())
}

type SourceStore interface {
	SaveSource(context.Context, string, io.Reader) error
	Delete(context.Context, string) error
}

var _ ObjectStore = (*LocalStore)(nil)
var _ SourceStore = (*LocalStore)(nil)

func NewLocalStore(root, stagingKey, mediaKey string) (*LocalStore, error) {
	if root == "" {
		return nil, fmt.Errorf("media storage directory is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create media storage directory: %w", err)
	}
	parsedStagingKey, err := crypto.ParseStagingKey(stagingKey)
	if err != nil {
		return nil, err
	}
	parsedMediaKey, err := crypto.ParseMasterKey(mediaKey)
	if err != nil {
		return nil, err
	}
	return &LocalStore{root: root, stagingKey: parsedStagingKey, mediaKey: parsedMediaKey}, nil
}

func (s *LocalStore) PutEncrypted(ctx context.Context, objectKey string, src io.Reader) (crypto.AtRestResult, error) {
	if err := ctx.Err(); err != nil {
		return crypto.AtRestResult{}, err
	}
	if len(s.mediaKey) != 32 {
		return crypto.AtRestResult{}, fmt.Errorf("media encryption key is required")
	}
	path := filepath.Join(s.root, objectKey+".bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("create encrypted media directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return crypto.AtRestResult{}, fmt.Errorf("create encrypted media object: %w", err)
	}
	result, encryptErr := crypto.EncryptToAtRest(file, src, s.mediaKey)
	closeErr := file.Close()
	if encryptErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return crypto.AtRestResult{}, errors.Join(encryptErr, closeErr)
	}
	return result, nil
}

func (s *LocalStore) OpenDecrypted(ctx context.Context, objectKey string, metadata crypto.AtRestResult, start, end int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if start < 0 || end < start || end > metadata.PlaintextSize {
		return nil, fmt.Errorf("media range is invalid")
	}
	offset, length, _, err := crypto.EncryptedRange(metadata, start, end)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(s.root, objectKey+".bin"))
	if err != nil {
		return nil, fmt.Errorf("open encrypted media object: %w", err)
	}
	section := io.NewSectionReader(file, offset, length)
	reader, writer := io.Pipe()
	go func() {
		decryptErr := crypto.DecryptRangeTo(writer, section, metadata, s.mediaKey, start, end)
		_ = file.Close()
		_ = writer.CloseWithError(decryptErr)
	}()
	return &pipeReadCloser{PipeReader: reader, closeFn: func() error { _ = file.Close(); return reader.Close() }}, nil
}

func (s *LocalStore) SaveSource(ctx context.Context, objectKey string, src io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(s.root, objectKey+".source")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create source media directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create source media object: %w", err)
	}
	if len(s.stagingKey) != 32 {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("staging encryption key is required")
	}
	copyErr := crypto.EncryptStagingTo(file, src, s.stagingKey)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write source media object: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close source media object: %w", closeErr)
	}
	return nil
}

func (s *LocalStore) Delete(_ context.Context, objectKey string) error {
	if err := os.Remove(filepath.Join(s.root, objectKey+".source")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete source media object: %w", err)
	}
	return nil
}
