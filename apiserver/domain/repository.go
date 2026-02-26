package domain

import (
	"context"
	"time"

	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
)

type VideoRepository interface {
	Create(ctx context.Context, video Video) error
	GetByID(ctx context.Context, videoID string) (*Video, error)
	ListByOwner(ctx context.Context, ownerUserID string, status *VideoStatus, p Pagination) (VideoConnection, error)
	ListByOwnerAndTag(ctx context.Context, ownerUserID string, tag string, status *VideoStatus, p Pagination) (VideoConnection, error)
	Update(ctx context.Context, video Video) error
	UpdateRating(ctx context.Context, videoID string, ownerUserID string, rating *Rating) error
	IncrementPlayCount(ctx context.Context, videoID string, playedAt synchro.Time[tz.UTC]) error
}

type VideoTagRepository interface {
	SetTags(ctx context.Context, videoID string, tags []string) error
	GetTags(ctx context.Context, videoID string) ([]string, error)
	GetTagsBatch(ctx context.Context, videoIDs []string) (map[string][]string, error)
}

type SharedKeyRepository interface {
	Register(ctx context.Context, userID string, publicKeyPEM string) (SharedKeyVersion, error)
	Revoke(ctx context.Context, userID string, version int) (SharedKeyVersion, error)
	List(ctx context.Context, userID string) ([]SharedKeyVersion, error)
	Get(ctx context.Context, userID string, version int) (*SharedKeyVersion, error)
}

type DeviceKeyRepository interface {
	RegisterWrappedSharedKey(ctx context.Context, userID string, data DeviceWrappedSharedKey) (DeviceWrappedSharedKey, error)
	ListWrappedSharedKeys(ctx context.Context, userID string) ([]DeviceWrappedSharedKey, error)
}

type UploadSessionRepository interface {
	Create(ctx context.Context, session UploadSession) error
	GetByID(ctx context.Context, id string) (*UploadSession, error)
	Delete(ctx context.Context, id string) error
}

type EncodingProgressRepository interface {
	Set(ctx context.Context, progress VideoEncodingProgress) error
	Get(ctx context.Context, videoID string) (*VideoEncodingProgress, error)
	Subscribe(ctx context.Context, videoID string) (<-chan VideoEncodingProgress, func(), error)
}

type ObjectStorage interface {
	CreateUploadURL(ctx context.Context, objectKey string, contentType string, expiresIn time.Duration) (string, error)
	Exists(ctx context.Context, objectKey string) (bool, error)
}

type PlaybackHistoryRepository interface {
	Record(ctx context.Context, history PlaybackHistory) error
	ListByVideo(ctx context.Context, videoID string, limit int) ([]PlaybackHistory, error)
}
