package media_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/walnuts1018/beast/backend/internal/media"
)

func TestLocalStoreDecryptsOnlyRequestedRange(t *testing.T) {
	root := t.TempDir()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	store, err := media.NewLocalStore(root, key, key)
	if err != nil {
		t.Fatal(err)
	}
	plain := strings.Repeat("private video ", 200000)
	metadata, err := store.PutEncrypted(context.Background(), "object", strings.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.OpenDecrypted(context.Background(), "object", metadata, 17, 400017)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != plain[17:400017] {
		t.Fatal("decrypted range differs from plaintext")
	}
}
