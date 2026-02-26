package domain

import "time"

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
	CreatedAt    time.Time
	RevokedAt    *time.Time
}

type DeviceWrappedSharedKey struct {
	DeviceID                  string
	DevicePublicKeyPEM        string
	SharedKeyVersion          int
	EncryptedSharedPrivateKey []byte
	CreatedAt                 time.Time
}

type UploadSession struct {
	ID        string
	OwnerUser string
	ObjectKey string
	UploadURL string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type VideoEncodingProgress struct {
	VideoID   string
	Status    VideoStatus
	Percent   float64
	UpdatedAt time.Time
	Message   *string
	OwnerUser string
}

type PlaybackGrant struct {
	VideoID     string
	ManifestURL string
	ExpiresAt   time.Time
	Encryption  EncryptionMetadata
}

type Video struct {
	ID                string
	OwnerUserID       string
	Status            VideoStatus
	UploadedAt        time.Time
	ReadyAt           *time.Time
	FailedReason      *string
	DurationMillis    *int
	Width             *int
	Height            *int
	Playback          *PlaybackGrant
	EncryptedTags     []byte
	TagEncryption     EncryptionMetadata
	ContentEncryption *EncryptionMetadata
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
