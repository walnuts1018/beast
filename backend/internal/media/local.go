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
	root string
}

type ObjectStore interface {
	Save(context.Context, string, io.Reader, string) (crypto.Result, error)
	Open(context.Context, string) (io.ReadCloser, error)
}

var _ ObjectStore = (*LocalStore)(nil)

func NewLocalStore(root string) (*LocalStore, error) {
	if root == "" {
		return nil, fmt.Errorf("media storage directory is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create media storage directory: %w", err)
	}
	return &LocalStore{root: root}, nil
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
