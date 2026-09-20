package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
)

const ChunkSize = 1 << 20

const Algorithm = "AES-256-GCM-CHUNKED-RSA-OAEP-SHA256"

type Result struct {
	Algorithm        string
	ChunkSize        int
	Nonce            string
	EncryptedDataKey string
}

func EncryptTo(dst io.Writer, src io.Reader, publicKeyPEM string) (Result, error) {
	publicKey, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return Result{}, err
	}
	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return Result{}, fmt.Errorf("generate data key: %w", err)
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Result{}, fmt.Errorf("generate nonce: %w", err)
	}
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return Result{}, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Result{}, fmt.Errorf("create gcm: %w", err)
	}
	plain := make([]byte, ChunkSize)
	var length [4]byte
	for index := uint64(0); ; index++ {
		read, readErr := io.ReadFull(src, plain)
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
			return Result{}, fmt.Errorf("read artifact: %w", readErr)
		}
		if read > 0 {
			sealed := gcm.Seal(nil, chunkNonce(nonce, index), plain[:read], nil)
			binary.BigEndian.PutUint32(length[:], uint32(len(sealed)))
			if _, err := dst.Write(length[:]); err != nil {
				return Result{}, fmt.Errorf("write encrypted length: %w", err)
			}
			if _, err := dst.Write(sealed); err != nil {
				return Result{}, fmt.Errorf("write encrypted chunk: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
	}
	encryptedDataKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, dataKey, nil)
	if err != nil {
		return Result{}, fmt.Errorf("encrypt data key: %w", err)
	}
	return Result{Algorithm: Algorithm, ChunkSize: ChunkSize, Nonce: base64.RawStdEncoding.EncodeToString(nonce), EncryptedDataKey: base64.RawStdEncoding.EncodeToString(encryptedDataKey)}, nil
}

func parsePublicKey(value string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("public key is not PEM encoded")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok || publicKey.Size() < 256 {
		return nil, errors.New("public key must be RSA with at least 2048 bits")
	}
	return publicKey, nil
}

func chunkNonce(base []byte, index uint64) []byte {
	nonce := append([]byte(nil), base...)
	for offset := range 8 {
		nonce[len(nonce)-1-offset] ^= byte(index >> (offset * 8))
	}
	return nonce
}
