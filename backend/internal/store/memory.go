package store

import (
	"context"
	"sync"
	"time"

	"github.com/walnuts1018/beast/backend/internal/domain"
)

type Memory struct {
	mu     sync.RWMutex
	videos map[string]domain.Video
}

var _ Repository = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{videos: make(map[string]domain.Video)}
}

func (m *Memory) CreateVideo(_ context.Context, video domain.Video) (domain.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.videos[video.ID] = video
	return video, nil
}

func (m *Memory) ListVideos(_ context.Context, ownerID string) ([]domain.Video, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	videos := make([]domain.Video, 0, len(m.videos))
	for _, video := range m.videos {
		if video.OwnerID == ownerID {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

func (m *Memory) GetVideo(_ context.Context, ownerID, id string) (domain.Video, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	video, ok := m.videos[id]
	if !ok || video.OwnerID != ownerID {
		return domain.Video{}, domain.ErrVideoNotFound
	}
	return video, nil
}

func (m *Memory) GetVideoByObjectKey(_ context.Context, ownerID, objectKey string) (domain.Video, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, video := range m.videos {
		if video.OwnerID == ownerID && video.ObjectKey == objectKey {
			return video, nil
		}
	}
	return domain.Video{}, domain.ErrVideoNotFound
}

func (m *Memory) UpdateVideoTags(_ context.Context, ownerID, id string, tags []string) (domain.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	video, ok := m.videos[id]
	if !ok || video.OwnerID != ownerID {
		return domain.Video{}, domain.ErrVideoNotFound
	}
	normalized, err := domain.NormalizeTags(tags)
	if err != nil {
		return domain.Video{}, err
	}
	video.Tags = normalized
	video.UpdatedAt = time.Now().UTC()
	m.videos[id] = video
	return video, nil
}

func (m *Memory) UpdateVideo(_ context.Context, video domain.Video) (domain.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.videos[video.ID]
	if !ok || current.OwnerID != video.OwnerID {
		return domain.Video{}, domain.ErrVideoNotFound
	}
	if video.HLSArtifacts == nil {
		video.HLSArtifacts = current.HLSArtifacts
	}
	m.videos[video.ID] = video
	return video, nil
}

func (m *Memory) RecordPlayback(_ context.Context, ownerID, id string) (domain.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	video, ok := m.videos[id]
	if !ok || video.OwnerID != ownerID {
		return domain.Video{}, domain.ErrVideoNotFound
	}
	video.RecordPlayback(time.Now())
	m.videos[id] = video
	return video, nil
}

func (m *Memory) SetRating(_ context.Context, ownerID, id string, rating *int) (domain.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	video, ok := m.videos[id]
	if !ok || video.OwnerID != ownerID {
		return domain.Video{}, domain.ErrVideoNotFound
	}
	if err := video.SetRating(rating); err != nil {
		return domain.Video{}, err
	}
	m.videos[id] = video
	return video, nil
}
