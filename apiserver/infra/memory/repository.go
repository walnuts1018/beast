package memory

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
	"github.com/walnuts1018/beast/apiserver/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	mu sync.RWMutex

	videos            map[string]domain.Video
	videoTags         map[string][]string
	sharedKeys        map[string][]domain.SharedKeyVersion
	deviceKeys        map[string][]domain.DeviceWrappedSharedKey
	uploadSessions    map[string]domain.UploadSession
	progress          map[string]domain.VideoEncodingProgress
	subscribers       map[string]map[chan domain.VideoEncodingProgress]struct{}
	playbackHistories map[string][]domain.PlaybackHistory
}

func NewStore() *Store {
	return &Store{
		videos:            make(map[string]domain.Video),
		videoTags:         make(map[string][]string),
		sharedKeys:        make(map[string][]domain.SharedKeyVersion),
		deviceKeys:        make(map[string][]domain.DeviceWrappedSharedKey),
		uploadSessions:    make(map[string]domain.UploadSession),
		progress:          make(map[string]domain.VideoEncodingProgress),
		subscribers:       make(map[string]map[chan domain.VideoEncodingProgress]struct{}),
		playbackHistories: make(map[string][]domain.PlaybackHistory),
	}
}

type VideoRepository struct{ store *Store }

func NewVideoRepository(store *Store) *VideoRepository {
	return &VideoRepository{store: store}
}

func (r *VideoRepository) Create(ctx context.Context, video domain.Video) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	r.store.videos[video.ID] = video
	return nil
}

func (r *VideoRepository) GetByID(ctx context.Context, videoID string) (*domain.Video, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	v, ok := r.store.videos[videoID]
	if !ok {
		return nil, nil
	}
	copyVideo := v
	return &copyVideo, nil
}

func (r *VideoRepository) ClaimNextUploadedForEncoding(ctx context.Context, now synchro.Time[tz.UTC]) (*domain.Video, error) {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	var picked *domain.Video
	for _, v := range r.store.videos {
		if v.Status != domain.VideoStatusUploaded {
			continue
		}
		if picked == nil || v.UploadedAt.Before(picked.UploadedAt) {
			copyVideo := v
			picked = &copyVideo
		}
	}

	if picked == nil {
		return nil, nil
	}

	picked.Status = domain.VideoStatusEncoding
	picked.FailedReason = nil
	picked.UpdatedAt = now
	r.store.videos[picked.ID] = *picked

	result := *picked
	return &result, nil
}

func (r *VideoRepository) ListByOwner(
	ctx context.Context,
	ownerUserID string,
	status *domain.VideoStatus,
	p domain.Pagination,
) (domain.VideoConnection, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	videos := make([]domain.Video, 0)
	for _, v := range r.store.videos {
		if v.OwnerUserID != ownerUserID {
			continue
		}
		if status != nil && v.Status != *status {
			continue
		}
		videos = append(videos, v)
	}

	sort.Slice(videos, func(i, j int) bool {
		return videos[i].UploadedAt.After(videos[j].UploadedAt)
	})

	start := 0
	if p.After != nil {
		for i := range videos {
			if videos[i].ID == *p.After {
				start = i + 1
				break
			}
		}
	}

	limit := p.First
	if limit <= 0 {
		limit = 20
	}
	end := start + limit
	hasNext := false
	if end < len(videos) {
		hasNext = true
	} else {
		end = len(videos)
	}

	edges := make([]domain.VideoEdge, 0, end-start)
	for _, video := range videos[start:end] {
		edges = append(edges, domain.VideoEdge{Cursor: video.ID, Node: video})
	}

	var next *string
	if len(edges) > 0 {
		last := edges[len(edges)-1].Cursor
		next = &last
	}

	return domain.VideoConnection{Edges: edges, HasNext: hasNext, NextCursor: next}, nil
}

func (r *VideoRepository) Update(ctx context.Context, video domain.Video) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	if _, ok := r.store.videos[video.ID]; !ok {
		return ErrNotFound
	}
	r.store.videos[video.ID] = video
	return nil
}

func (r *VideoRepository) ListByOwnerAndTag(
	ctx context.Context,
	ownerUserID string,
	tag string,
	status *domain.VideoStatus,
	p domain.Pagination,
) (domain.VideoConnection, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	// タグが一致する動画IDを収集
	taggedVideoIDs := make(map[string]struct{})
	for videoID, tags := range r.store.videoTags {
		for _, t := range tags {
			if t == tag {
				taggedVideoIDs[videoID] = struct{}{}
				break
			}
		}
	}

	videos := make([]domain.Video, 0)
	for _, v := range r.store.videos {
		if v.OwnerUserID != ownerUserID {
			continue
		}
		if _, ok := taggedVideoIDs[v.ID]; !ok {
			continue
		}
		if status != nil && v.Status != *status {
			continue
		}
		videos = append(videos, v)
	}

	sort.Slice(videos, func(i, j int) bool {
		return videos[i].UploadedAt.After(videos[j].UploadedAt)
	})

	start := 0
	if p.After != nil {
		for i := range videos {
			if videos[i].ID == *p.After {
				start = i + 1
				break
			}
		}
	}

	limit := p.First
	if limit <= 0 {
		limit = 20
	}
	end := start + limit
	hasNext := false
	if end < len(videos) {
		hasNext = true
	} else {
		end = len(videos)
	}

	edges := make([]domain.VideoEdge, 0, end-start)
	for _, video := range videos[start:end] {
		edges = append(edges, domain.VideoEdge{Cursor: video.ID, Node: video})
	}

	var next *string
	if len(edges) > 0 {
		last := edges[len(edges)-1].Cursor
		next = &last
	}

	return domain.VideoConnection{Edges: edges, HasNext: hasNext, NextCursor: next}, nil
}

func (r *VideoRepository) UpdateRating(ctx context.Context, videoID string, ownerUserID string, rating *domain.Rating) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	v, ok := r.store.videos[videoID]
	if !ok || v.OwnerUserID != ownerUserID {
		return ErrNotFound
	}
	v.Rating = rating
	r.store.videos[videoID] = v
	return nil
}

func (r *VideoRepository) IncrementPlayCount(ctx context.Context, videoID string, playedAt synchro.Time[tz.UTC]) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	v, ok := r.store.videos[videoID]
	if !ok {
		return ErrNotFound
	}
	v.PlayCount++
	v.LastPlayedAt = &playedAt
	r.store.videos[videoID] = v
	return nil
}

type VideoTagRepository struct{ store *Store }

func NewVideoTagRepository(store *Store) *VideoTagRepository {
	return &VideoTagRepository{store: store}
}

func (r *VideoTagRepository) SetTags(ctx context.Context, videoID string, tags []string) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	copied := make([]string, len(tags))
	copy(copied, tags)
	sort.Strings(copied)
	r.store.videoTags[videoID] = copied
	return nil
}

func (r *VideoTagRepository) GetTags(ctx context.Context, videoID string) ([]string, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	src := r.store.videoTags[videoID]
	out := make([]string, len(src))
	copy(out, src)
	return out, nil
}

func (r *VideoTagRepository) GetTagsBatch(ctx context.Context, videoIDs []string) (map[string][]string, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	result := make(map[string][]string, len(videoIDs))
	for _, id := range videoIDs {
		src := r.store.videoTags[id]
		out := make([]string, len(src))
		copy(out, src)
		result[id] = out
	}
	return result, nil
}

type SharedKeyRepository struct{ store *Store }

func NewSharedKeyRepository(store *Store) *SharedKeyRepository {
	return &SharedKeyRepository{store: store}
}

func (r *SharedKeyRepository) Register(ctx context.Context, userID string, publicKeyPEM string) (domain.SharedKeyVersion, error) {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	versions := r.store.sharedKeys[userID]
	version := len(versions) + 1
	item := domain.SharedKeyVersion{
		Version:      version,
		PublicKeyPEM: publicKeyPEM,
		Status:       domain.SharedKeyVersionStatusActive,
		CreatedAt:    synchro.Now[tz.UTC](),
	}

	r.store.sharedKeys[userID] = append(versions, item)
	return item, nil
}

func (r *SharedKeyRepository) Revoke(ctx context.Context, userID string, version int) (domain.SharedKeyVersion, error) {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	versions := r.store.sharedKeys[userID]
	for i := range versions {
		if versions[i].Version == version {
			now := synchro.Now[tz.UTC]()
			versions[i].Status = domain.SharedKeyVersionStatusRevoked
			versions[i].RevokedAt = &now
			r.store.sharedKeys[userID] = versions
			return versions[i], nil
		}
	}

	return domain.SharedKeyVersion{}, ErrNotFound
}

func (r *SharedKeyRepository) List(ctx context.Context, userID string) ([]domain.SharedKeyVersion, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	src := r.store.sharedKeys[userID]
	out := make([]domain.SharedKeyVersion, len(src))
	copy(out, src)
	return out, nil
}

func (r *SharedKeyRepository) Get(ctx context.Context, userID string, version int) (*domain.SharedKeyVersion, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, v := range r.store.sharedKeys[userID] {
		if v.Version == version {
			copyVersion := v
			return &copyVersion, nil
		}
	}

	return nil, ErrNotFound
}

type DeviceKeyRepository struct{ store *Store }

func NewDeviceKeyRepository(store *Store) *DeviceKeyRepository {
	return &DeviceKeyRepository{store: store}
}

func (r *DeviceKeyRepository) RegisterWrappedSharedKey(
	ctx context.Context,
	userID string,
	data domain.DeviceWrappedSharedKey,
) (domain.DeviceWrappedSharedKey, error) {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	// PostgreSQLのON CONFLICTと同じくupsert動作にする
	existing := r.store.deviceKeys[userID]
	for i, item := range existing {
		if item.DeviceID == data.DeviceID {
			existing[i] = data
			r.store.deviceKeys[userID] = existing
			return data, nil
		}
	}

	r.store.deviceKeys[userID] = append(existing, data)
	return data, nil
}

func (r *DeviceKeyRepository) ListWrappedSharedKeys(ctx context.Context, userID string) ([]domain.DeviceWrappedSharedKey, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	src := r.store.deviceKeys[userID]
	out := make([]domain.DeviceWrappedSharedKey, len(src))
	copy(out, src)
	return out, nil
}

type UploadSessionRepository struct{ store *Store }

func NewUploadSessionRepository(store *Store) *UploadSessionRepository {
	return &UploadSessionRepository{store: store}
}

func (r *UploadSessionRepository) Create(ctx context.Context, session domain.UploadSession) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	r.store.uploadSessions[session.ID] = session
	return nil
}

func (r *UploadSessionRepository) GetByID(ctx context.Context, id string) (*domain.UploadSession, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	v, ok := r.store.uploadSessions[id]
	if !ok {
		return nil, nil
	}

	copySession := v
	return &copySession, nil
}

func (r *UploadSessionRepository) Delete(ctx context.Context, id string) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	delete(r.store.uploadSessions, id)
	return nil
}

type EncodingProgressRepository struct{ store *Store }

func NewEncodingProgressRepository(store *Store) *EncodingProgressRepository {
	return &EncodingProgressRepository{store: store}
}

func (r *EncodingProgressRepository) Set(ctx context.Context, progress domain.VideoEncodingProgress) error {
	_ = ctx
	r.store.mu.Lock()
	r.store.progress[progress.VideoID] = progress

	subscribers := r.store.subscribers[progress.VideoID]
	for ch := range subscribers {
		select {
		case ch <- progress:
		default:
		}
	}
	r.store.mu.Unlock()

	return nil
}

func (r *EncodingProgressRepository) Get(ctx context.Context, videoID string) (*domain.VideoEncodingProgress, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	v, ok := r.store.progress[videoID]
	if !ok {
		return nil, nil
	}

	copyProgress := v
	return &copyProgress, nil
}

func (r *EncodingProgressRepository) Subscribe(
	ctx context.Context,
	videoID string,
) (<-chan domain.VideoEncodingProgress, func(), error) {
	ch := make(chan domain.VideoEncodingProgress, 8)

	r.store.mu.Lock()
	if _, ok := r.store.subscribers[videoID]; !ok {
		r.store.subscribers[videoID] = make(map[chan domain.VideoEncodingProgress]struct{})
	}
	r.store.subscribers[videoID][ch] = struct{}{}
	r.store.mu.Unlock()

	cleanupOnce := sync.Once{}
	cleanup := func() {
		cleanupOnce.Do(func() {
			r.store.mu.Lock()
			if subscribers, ok := r.store.subscribers[videoID]; ok {
				delete(subscribers, ch)
			}
			r.store.mu.Unlock()
			close(ch)
		})
	}

	go func() {
		<-ctx.Done()
		cleanup()
	}()

	return ch, cleanup, nil
}

// PlaybackHistoryRepository

type PlaybackHistoryRepository struct{ store *Store }

func NewPlaybackHistoryRepository(store *Store) *PlaybackHistoryRepository {
	return &PlaybackHistoryRepository{store: store}
}

func (r *PlaybackHistoryRepository) Record(ctx context.Context, history domain.PlaybackHistory) error {
	_ = ctx
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	r.store.playbackHistories[history.VideoID] = append(r.store.playbackHistories[history.VideoID], history)
	return nil
}

func (r *PlaybackHistoryRepository) ListByVideo(ctx context.Context, videoID string, limit int) ([]domain.PlaybackHistory, error) {
	_ = ctx
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	src := r.store.playbackHistories[videoID]
	out := make([]domain.PlaybackHistory, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool { return out[i].PlayedAt.After(out[j].PlayedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
