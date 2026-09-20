package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/walnuts1018/beast/backend/internal/domain"
)

type SharedKey struct {
	ID        string
	OwnerID   string
	Version   string
	PublicKey string
	Status    string
}

type DeviceKey struct {
	ID                        string
	OwnerID                   string
	DeviceID                  string
	SharedKeyID               string
	EncryptedSharedPrivateKey string
}

type Memory struct {
	mu         sync.RWMutex
	videos     map[string]domain.Video
	sharedKeys map[string]SharedKey
	deviceKeys map[string]DeviceKey
}

var _ Repository = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{videos: make(map[string]domain.Video), sharedKeys: make(map[string]SharedKey), deviceKeys: make(map[string]DeviceKey)}
}

func (m *Memory) RegisterSharedKey(_ context.Context, key SharedKey) (SharedKey, error) {
	if key.ID == "" || key.OwnerID == "" || key.Version == "" || key.PublicKey == "" {
		return SharedKey{}, errors.New("shared key fields are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sharedKeys[key.ID] = key
	return key, nil
}

func (m *Memory) SharedKey(_ context.Context, ownerID, id string) (SharedKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key, ok := m.sharedKeys[id]
	if !ok || key.OwnerID != ownerID {
		return SharedKey{}, domain.ErrVideoNotFound
	}
	return key, nil
}

func (m *Memory) ListSharedKeys(_ context.Context, ownerID string) ([]SharedKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]SharedKey, 0, len(m.sharedKeys))
	for _, key := range m.sharedKeys {
		if key.OwnerID == ownerID {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *Memory) RegisterDeviceKey(_ context.Context, key DeviceKey) (DeviceKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sharedKey, ok := m.sharedKeys[key.SharedKeyID]
	if !ok || sharedKey.OwnerID != key.OwnerID {
		return DeviceKey{}, domain.ErrVideoNotFound
	}
	m.deviceKeys[key.ID] = key
	return key, nil
}

func (m *Memory) ListDeviceKeys(_ context.Context, ownerID string) ([]DeviceKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]DeviceKey, 0, len(m.deviceKeys))
	for _, key := range m.deviceKeys {
		if key.OwnerID == ownerID {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *Memory) CreateVideo(_ context.Context, video domain.Video) (domain.Video, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sharedKeys[video.Encryption.SharedKeyID]; !ok {
		return domain.Video{}, errors.New("shared key is not registered")
	}
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
