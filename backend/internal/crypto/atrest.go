package crypto

import (
	"bytes"
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

// Resultは暗号化されたオブジェクトをRange復号するための公開メタデータです。
type AtRestResult struct {
	Algorithm        string
	ChunkSize        int
	Nonce            string
	EncryptedDataKey string
	PlaintextSize    int64
}

func EncryptTags(value []byte, key []byte) (ciphertext, nonce []byte, err error) {
	gcm, err := newKeyedGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate tags nonce: %w", err)
	}
	return gcm.Seal(nil, nonce, value, nil), nonce, nil
}

func DecryptTags(ciphertext, nonce, key []byte) ([]byte, error) {
	gcm, err := newKeyedGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("tags nonce is invalid")
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
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
	gcm, err := newKeyedGCM(dataKey)
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

func DecryptRange(src io.Reader, metadata AtRestResult, masterKey []byte, start, end int64) ([]byte, error) {
	if err := validateRange(metadata, masterKey, start, end); err != nil {
		return nil, err
	}
	if start == end {
		return []byte{}, nil
	}
	nonce, err := decodeNonce(metadata.Nonce)
	if err != nil {
		return nil, err
	}
	dataKey, err := unwrapDataKey(masterKey, metadata.EncryptedDataKey)
	if err != nil {
		return nil, err
	}
	gcm, err := newKeyedGCM(dataKey)
	if err != nil {
		return nil, err
	}
	first := uint64(start / int64(metadata.ChunkSize))
	last := uint64((end - 1) / int64(metadata.ChunkSize))
	result := make([]byte, 0, end-start)
	for index := first; index <= last; index++ {
		plainSize := chunkPlaintextSize(metadata.PlaintextSize, metadata.ChunkSize, index)
		sealed := make([]byte, plainSize+int64(gcm.Overhead()))
		if _, err := io.ReadFull(src, sealed); err != nil {
			return nil, fmt.Errorf("read encrypted media chunk: %w", err)
		}
		plain, err := gcm.Open(nil, atRestChunkNonce(nonce, index), sealed, nil)
		if err != nil {
			return nil, fmt.Errorf("decrypt media chunk: %w", err)
		}
		chunkStart := int64(index) * int64(metadata.ChunkSize)
		from := maxInt64(start, chunkStart) - chunkStart
		to := minInt64(end, chunkStart+int64(len(plain))) - chunkStart
		if from < 0 || to < from || to > int64(len(plain)) {
			return nil, errors.New("decrypted media range is invalid")
		}
		result = append(result, plain[from:to]...)
	}
	return result, nil
}

func DecryptRangeTo(dst io.Writer, src io.Reader, metadata AtRestResult, masterKey []byte, start, end int64) error {
	if err := validateRange(metadata, masterKey, start, end); err != nil {
		return err
	}
	if start == end {
		return nil
	}
	nonce, err := decodeNonce(metadata.Nonce)
	if err != nil {
		return err
	}
	dataKey, err := unwrapDataKey(masterKey, metadata.EncryptedDataKey)
	if err != nil {
		return err
	}
	gcm, err := newKeyedGCM(dataKey)
	if err != nil {
		return err
	}
	first := uint64(start / int64(metadata.ChunkSize))
	last := uint64((end - 1) / int64(metadata.ChunkSize))
	for index := first; index <= last; index++ {
		plainSize := chunkPlaintextSize(metadata.PlaintextSize, metadata.ChunkSize, index)
		sealed := make([]byte, plainSize+int64(gcm.Overhead()))
		if _, err := io.ReadFull(src, sealed); err != nil {
			return fmt.Errorf("read encrypted media chunk: %w", err)
		}
		plain, err := gcm.Open(nil, atRestChunkNonce(nonce, index), sealed, nil)
		if err != nil {
			return fmt.Errorf("decrypt media chunk: %w", err)
		}
		chunkStart := int64(index) * int64(metadata.ChunkSize)
		from := maxInt64(start, chunkStart) - chunkStart
		to := minInt64(end, chunkStart+int64(len(plain))) - chunkStart
		if from < 0 || to < from || to > int64(len(plain)) {
			return errors.New("decrypted media range is invalid")
		}
		if _, err := dst.Write(plain[from:to]); err != nil {
			return fmt.Errorf("write decrypted media range: %w", err)
		}
	}
	return nil
}

func EncryptedRange(metadata AtRestResult, start, end int64) (offset, length int64, firstChunk uint64, err error) {
	if err := validateMetadata(metadata); err != nil {
		return 0, 0, 0, err
	}
	if start < 0 || end < start || end > metadata.PlaintextSize {
		return 0, 0, 0, errors.New("media range is invalid")
	}
	if start == end {
		return int64(len(atRestMagic) + 12), 0, uint64(start / int64(metadata.ChunkSize)), nil
	}
	first := uint64(start / int64(metadata.ChunkSize))
	last := uint64((end - 1) / int64(metadata.ChunkSize))
	chunkWireSize := int64(metadata.ChunkSize + 16)
	offset = int64(len(atRestMagic)+12) + int64(first)*chunkWireSize
	lastSize := chunkPlaintextSize(metadata.PlaintextSize, metadata.ChunkSize, last) + 16
	lastOffset := int64(len(atRestMagic)+12) + int64(last)*chunkWireSize
	return offset, lastOffset + lastSize - offset, first, nil
}

func validateRange(metadata AtRestResult, masterKey []byte, start, end int64) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	if len(masterKey) != 32 {
		return errors.New("media encryption key must be 32 bytes")
	}
	if start < 0 || end < start || end > metadata.PlaintextSize {
		return errors.New("media range is invalid")
	}
	return nil
}

func validateMetadata(metadata AtRestResult) error {
	if metadata.Algorithm != AtRestAlgorithm {
		return errors.New("unsupported media encryption algorithm")
	}
	if metadata.ChunkSize != AtRestChunkSize || metadata.PlaintextSize < 0 {
		return errors.New("invalid media encryption metadata")
	}
	if _, err := decodeNonce(metadata.Nonce); err != nil {
		return err
	}
	if metadata.EncryptedDataKey == "" {
		return errors.New("encrypted media data key is required")
	}
	return nil
}

func chunkPlaintextSize(total int64, chunkSize int, index uint64) int64 {
	start := int64(index) * int64(chunkSize)
	if start >= total {
		return 0
	}
	return minInt64(int64(chunkSize), total-start)
}

func wrapDataKey(master, dataKey []byte) ([]byte, error) {
	gcm, err := newKeyedGCM(master)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate media key wrapping nonce: %w", err)
	}
	return append(nonce, gcm.Seal(nil, nonce, dataKey, nil)...), nil
}

func unwrapDataKey(master []byte, encoded string) ([]byte, error) {
	wrapped, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted media data key: %w", err)
	}
	gcm, err := newKeyedGCM(master)
	if err != nil {
		return nil, err
	}
	if len(wrapped) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("encrypted media data key is truncated")
	}
	return gcm.Open(nil, wrapped[:gcm.NonceSize()], wrapped[gcm.NonceSize():], nil)
}

func decodeNonce(encoded string) ([]byte, error) {
	nonce, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(nonce) != 12 {
		return nil, errors.New("media nonce must be 12 bytes in base64")
	}
	return nonce, nil
}

func newKeyedGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create media cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create media GCM: %w", err)
	}
	return gcm, nil
}

func atRestChunkNonce(base []byte, index uint64) []byte {
	nonce := bytes.Clone(base)
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], index)
	for i := range encoded {
		nonce[len(nonce)-len(encoded)+i] ^= encoded[i]
	}
	return nonce
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
