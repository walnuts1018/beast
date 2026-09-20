package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	AtRestAlgorithm = "AES-256-GCM-CHUNKED-SERVER-DEK-V1"
	AtRestChunkSize = 1 << 20
)

var atRestMagic = []byte("BEASTMED2")

type AtRestResult struct {
	Algorithm        string
	ChunkSize        int
	Nonce            string
	EncryptedDataKey string
	PlaintextSize    int64
}

func ParseMasterKey(value string) ([]byte, error) {
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, errors.New("media encryption key must be 32 bytes in base64 or hexadecimal")
}

func EncryptToAtRest(dst io.Writer, src io.Reader, masterKey []byte) (AtRestResult, error) {
	if len(masterKey) != 32 {
		return AtRestResult{}, errors.New("media encryption key must be 32 bytes")
	}
	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return AtRestResult{}, fmt.Errorf("generate media data key: %w", err)
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return AtRestResult{}, fmt.Errorf("generate media nonce: %w", err)
	}
	gcm, err := keyedGCM(dataKey)
	if err != nil {
		return AtRestResult{}, err
	}
	if _, err := dst.Write(atRestMagic); err != nil {
		return AtRestResult{}, fmt.Errorf("write media header: %w", err)
	}
	if _, err := dst.Write(nonce); err != nil {
		return AtRestResult{}, fmt.Errorf("write media nonce: %w", err)
	}
	plain := make([]byte, AtRestChunkSize)
	var size int64
	for index := uint64(0); ; index++ {
		read, readErr := io.ReadFull(src, plain)
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
			return AtRestResult{}, fmt.Errorf("read media object: %w", readErr)
		}
		if read > 0 {
			sealed := gcm.Seal(nil, atRestChunkNonce(nonce, index), plain[:read], nil)
			if _, err := dst.Write(sealed); err != nil {
				return AtRestResult{}, fmt.Errorf("write media chunk: %w", err)
			}
			size += int64(read)
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
	}
	wrapped, err := wrapDataKey(masterKey, dataKey)
	if err != nil {
		return AtRestResult{}, err
	}
	return AtRestResult{Algorithm: AtRestAlgorithm, ChunkSize: AtRestChunkSize, Nonce: base64.RawStdEncoding.EncodeToString(nonce), EncryptedDataKey: base64.RawStdEncoding.EncodeToString(wrapped), PlaintextSize: size}, nil
}

func keyedGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create media cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func wrapDataKey(master, dataKey []byte) ([]byte, error) {
	gcm, err := keyedGCM(master)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate media key wrapping nonce: %w", err)
	}
	return append(nonce, gcm.Seal(nil, nonce, dataKey, nil)...), nil
}

func atRestChunkNonce(base []byte, index uint64) []byte {
	nonce := append([]byte(nil), base...)
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], index)
	for i := range encoded {
		nonce[len(nonce)-len(encoded)+i] ^= encoded[i]
	}
	return nonce
}
