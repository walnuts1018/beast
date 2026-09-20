package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var stagingMagic = []byte("BEASTSTG1")

func ParseStagingKey(value string) ([]byte, error) {
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, errors.New("staging encryption key must be 32 bytes in base64 or hexadecimal")
}

func DecryptStagingTo(dst io.Writer, src io.Reader, key []byte) error {
	header := make([]byte, len(stagingMagic))
	if _, err := io.ReadFull(src, header); err != nil {
		return fmt.Errorf("read staging header: %w", err)
	}
	if string(header) != string(stagingMagic) {
		return errors.New("staging object header is invalid")
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(src, nonce); err != nil {
		return fmt.Errorf("read staging nonce: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	var length [4]byte
	for index := uint64(0); ; index++ {
		if _, err := io.ReadFull(src, length[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read staging chunk length: %w", err)
		}
		size := binary.BigEndian.Uint32(length[:])
		if size < uint32(gcm.Overhead()) || size > uint32(ChunkSize+gcm.Overhead()) {
			return errors.New("staging chunk length is invalid")
		}
		sealed := make([]byte, size)
		if _, err := io.ReadFull(src, sealed); err != nil {
			return fmt.Errorf("read staging chunk: %w", err)
		}
		plain, err := gcm.Open(nil, chunkNonce(nonce, index), sealed, nil)
		if err != nil {
			return fmt.Errorf("decrypt staging chunk: %w", err)
		}
		if _, err := dst.Write(plain); err != nil {
			return fmt.Errorf("write decrypted staging source: %w", err)
		}
	}
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("staging encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create staging cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create staging GCM: %w", err)
	}
	return gcm, nil
}
