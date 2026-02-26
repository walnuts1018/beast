package domain

import (
	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
)

type VideoStatus string

const (
	VideoStatusUploaded VideoStatus = "UPLOADED"
	VideoStatusEncoding VideoStatus = "ENCODING"
	VideoStatusReady    VideoStatus = "READY"
	VideoStatusFailed   VideoStatus = "FAILED"
)

type EncryptionAlgorithm string

const (
	EncryptionAlgorithmXChaCha20Poly1305 EncryptionAlgorithm = "XCHACHA20_POLY1305"
)

type EncryptionMetadata struct {
	Algorithm        EncryptionAlgorithm
	KeyVersion       int
	Nonce            []byte
	EncryptedDataKey []byte
}

type SharedKeyVersionStatus string

const (
	SharedKeyVersionStatusActive  SharedKeyVersionStatus = "ACTIVE"
	SharedKeyVersionStatusRevoked SharedKeyVersionStatus = "REVOKED"
)

type SharedKeyVersion struct {
	Version      int
	PublicKeyPEM string
	Status       SharedKeyVersionStatus
	CreatedAt    synchro.Time[tz.UTC]
	RevokedAt    *synchro.Time[tz.UTC]
}

type DeviceWrappedSharedKey struct {
	DeviceID                  string
	DevicePublicKeyPEM        string
	SharedKeyVersion          int
	EncryptedSharedPrivateKey []byte
	CreatedAt                 synchro.Time[tz.UTC]
}

type UploadSession struct {
	ID        string
	OwnerUser string
	ObjectKey string
	UploadURL string
	ExpiresAt synchro.Time[tz.UTC]
	CreatedAt synchro.Time[tz.UTC]
}

type VideoEncodingProgress struct {
	VideoID   string
	Status    VideoStatus
	Percent   float64
	UpdatedAt synchro.Time[tz.UTC]
	Message   *string
	OwnerUser string
}

type PlaybackGrant struct {
	VideoID     string
	ManifestURL string
	ExpiresAt   synchro.Time[tz.UTC]
	Encryption  EncryptionMetadata
}

type Video struct {
	ID                string
	OwnerUserID       string
	Status            VideoStatus
	UploadedAt        synchro.Time[tz.UTC]
	ReadyAt           *synchro.Time[tz.UTC]
	FailedReason      *string
	DurationMillis    *int
	Width             *int
	Height            *int
	Playback          *PlaybackGrant
	Tags              []string
	ContentEncryption *EncryptionMetadata
	CreatedAt         synchro.Time[tz.UTC]
	UpdatedAt         synchro.Time[tz.UTC]
}

type Pagination struct {
	After *string
	First int
}

type VideoConnection struct {
	Edges      []VideoEdge
	HasNext    bool
	NextCursor *string
}

type VideoEdge struct {
	Cursor string
	Node   Video
}

type Me struct {
	UserID                  string
	SharedKeyVersions       []SharedKeyVersion
	DeviceWrappedSharedKeys []DeviceWrappedSharedKey
}
