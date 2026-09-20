package crypto

import (
	"bytes"
	"io"
	"testing"
)

func TestAtRestRangeRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	plain := bytes.Repeat([]byte("beast-video"), AtRestChunkSize/len("beast-video")+137)
	var encrypted bytes.Buffer
	metadata, err := EncryptToAtRest(&encrypted, bytes.NewReader(plain), key)
	if err != nil {
		t.Fatal(err)
	}
	offset, length, _, err := EncryptedRange(metadata, 123, int64(len(plain)-17))
	if err != nil {
		t.Fatal(err)
	}
	chunked := bytes.NewReader(encrypted.Bytes()[offset : offset+length])
	got, err := DecryptRange(chunked, metadata, key, 123, int64(len(plain)-17))
	if err != nil {
		t.Fatal(err)
	}
	want := plain[123 : len(plain)-17]
	if !bytes.Equal(got, want) {
		t.Fatal("decrypted range differs from plaintext")
	}
	var streamed bytes.Buffer
	offset, length, _, err = EncryptedRange(metadata, 123, 4567)
	if err != nil {
		t.Fatal(err)
	}
	if err := DecryptRangeTo(&streamed, bytes.NewReader(encrypted.Bytes()[offset:offset+length]), metadata, key, 123, 4567); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, bytes.NewReader(streamed.Bytes())); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(streamed.Bytes(), plain[123:4567]) {
		t.Fatal("streamed decrypted range differs from plaintext")
	}
}
