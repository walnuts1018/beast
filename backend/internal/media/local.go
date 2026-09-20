package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/walnuts1018/beast/backend/internal/crypto"
)

type LocalStore struct {
	root       string
	stagingKey []byte
}

type ObjectStore interface {
	Save(context.Context, string, io.Reader, string) (crypto.Result, error)
	Open(context.Context, string) (io.ReadCloser, error)
}

type SourceStore interface {
	SaveSource(context.Context, string, io.Reader) error
	Delete(context.Context, string) error
}

var _ ObjectStore = (*LocalStore)(nil)
var _ SourceStore = (*LocalStore)(nil)

func NewLocalStore(root string, stagingKey ...string) (*LocalStore, error) {
	if root == "" {
		return nil, fmt.Errorf("media storage directory is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create media storage directory: %w", err)
	}
	var parsedKey []byte
	if len(stagingKey) > 0 && stagingKey[0] != "" {
		var err error
		parsedKey, err = crypto.ParseStagingKey(stagingKey[0])
		if err != nil {
			return nil, err
		}
	}
	return &LocalStore{root: root, stagingKey: parsedKey}, nil
}

func (s *LocalStore) Save(ctx context.Context, objectKey string, src io.Reader, publicKey string) (crypto.Result, error) {
	if err := ctx.Err(); err != nil {
		return crypto.Result{}, err
	}
	path := filepath.Join(s.root, objectKey+".bin")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return crypto.Result{}, fmt.Errorf("create encrypted media object: %w", err)
	}
	result, encryptErr := crypto.EncryptTo(file, src, publicKey)
	if closeErr := file.Close(); closeErr != nil && encryptErr == nil {
		encryptErr = fmt.Errorf("close encrypted media object: %w", closeErr)
	}
	if encryptErr != nil {
		_ = os.Remove(path)
		return crypto.Result{}, encryptErr
	}
	return result, nil
}

func (s *LocalStore) Open(_ context.Context, objectKey string) (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(s.root, objectKey+".bin"))
	if err != nil {
		return nil, fmt.Errorf("open encrypted media object: %w", err)
	}
	return file, nil
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
