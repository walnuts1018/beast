package store

import (
	"context"

	"github.com/walnuts1018/beast/backend/internal/domain"
)

type Repository interface {
	CreateVideo(context.Context, domain.Video) (domain.Video, error)
	ListVideos(context.Context, string) ([]domain.Video, error)
	GetVideo(context.Context, string, string) (domain.Video, error)
	GetVideoByObjectKey(context.Context, string, string) (domain.Video, error)
	UpdateVideoTags(context.Context, string, string, []string) (domain.Video, error)
	UpdateVideo(context.Context, domain.Video) (domain.Video, error)
	RecordPlayback(context.Context, string, string) (domain.Video, error)
	SetRating(context.Context, string, string, *int) (domain.Video, error)
}
