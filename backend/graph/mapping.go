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
	progress := video.Progress
	videoURL := ""
	if video.Status == domain.VideoStatusReady {
		videoURL = "/api/videos/" + video.ID + "/hls/manifest.m3u8"
	}
	return &model.Video{ID: video.ID, Status: model.VideoStatus(video.Status), Tags: append([]string(nil), video.Tags...), PlayCount: int(video.PlayCount), Rating: video.Rating, LastPlayedAt: lastPlayedAt, VideoURL: videoURL, Progress: progress}
}
