package graph

import (
	"time"

	"github.com/walnuts1018/beast/backend/graph/model"
	"github.com/walnuts1018/beast/backend/internal/domain"
)

func videoModel(video domain.Video) *model.Video {
	var lastPlayedAt *time.Time
	if video.LastPlayedAt != nil {
		lastPlayedAt = video.LastPlayedAt
	}
	progress := 0.0
	if video.Status == domain.VideoStatusReady {
		progress = 1
	}
	return &model.Video{
		ID: video.ID, Status: model.VideoStatus(video.Status), EncryptedTags: video.EncryptedTags,
		PlayCount: int(video.PlayCount), Rating: video.Rating, LastPlayedAt: lastPlayedAt,
		VideoURL: "/api/videos/" + video.ID + "/stream", Progress: progress,
		Encryption: &model.EncryptionMetadata{Algorithm: video.Encryption.Algorithm, ChunkSize: video.Encryption.ChunkSize, KeyVersion: video.Encryption.KeyVersion, Nonce: video.Encryption.Nonce, EncryptedDataKey: video.Encryption.EncryptedDataKey, SharedKeyID: video.Encryption.SharedKeyID},
	}
}
