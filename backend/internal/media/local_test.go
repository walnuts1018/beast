package media_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/walnuts1018/beast/backend/internal/media"
)

func TestLocalStoreEncryptsUpload(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyDER}))
	root := t.TempDir()
	store, err := media.NewLocalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Save(context.Background(), "object", strings.NewReader("private video"), publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if result.Algorithm == "" || len(result.EncryptedDataKey) == 0 {
		t.Fatal("encryption metadata is incomplete")
	}
	file, err := store.Open(context.Background(), "object")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	contents, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) == "private video" || len(contents) == 0 {
		t.Fatal("object was not encrypted")
	}
	if _, err := os.Stat(root + "/object.bin"); err != nil {
		t.Fatal(err)
	}
}
