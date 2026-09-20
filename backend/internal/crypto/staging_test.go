package crypto

import (
	"bytes"
	"strings"
	"testing"
)

func TestStagingEncryptionRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	plain := strings.Repeat("video-source-", ChunkSize/len("video-source-")+17)
	var encrypted bytes.Buffer
	if err := EncryptStagingTo(&encrypted, strings.NewReader(plain), key); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted.Bytes(), []byte("video-source-")) {
		t.Fatal("staging object contains plaintext")
	}
	var decrypted bytes.Buffer
	if err := DecryptStagingTo(&decrypted, &encrypted, key); err != nil {
		t.Fatal(err)
	}
	if decrypted.String() != plain {
		t.Fatal("staging round trip changed source")
	}
}
