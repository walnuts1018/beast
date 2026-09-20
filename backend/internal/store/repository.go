package store

import (
	"context"

	"github.com/walnuts1018/beast/backend/internal/domain"
)

type Repository interface {
	RegisterSharedKey(context.Context, SharedKey) (SharedKey, error)
	SharedKey(context.Context, string, string) (SharedKey, error)
	ListSharedKeys(context.Context, string) ([]SharedKey, error)
	RegisterDeviceKey(context.Context, DeviceKey) (DeviceKey, error)
	ListDeviceKeys(context.Context, string) ([]DeviceKey, error)
	CreateVideo(context.Context, domain.Video) (domain.Video, error)
	ListVideos(context.Context, string) ([]domain.Video, error)
	GetVideo(context.Context, string, string) (domain.Video, error)
	RecordPlayback(context.Context, string, string) (domain.Video, error)
	SetRating(context.Context, string, string, *int) (domain.Video, error)
}
