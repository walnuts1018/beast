package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
	"github.com/google/uuid"
	"github.com/walnuts1018/beast/apiserver/domain"
)

var (
	ErrUnauthorized        = errors.New("unauthorized")
	ErrNotFound            = errors.New("not found")
	ErrInvalidInput        = errors.New("invalid input")
	ErrUploadSessionGone   = errors.New("upload session expired or missing")
	ErrUploadObjectMissing = errors.New("uploaded object is missing")
	ErrInvalidStateChange  = errors.New("invalid video state transition")
	ErrAlreadyRevoked      = errors.New("key version already revoked")
)

type Service struct {
	videos       domain.VideoRepository
	tags         domain.VideoTagRepository
	sharedKeys   domain.SharedKeyRepository
	deviceKeys   domain.DeviceKeyRepository
	uploads      domain.UploadSessionRepository
	progress     domain.EncodingProgressRepository
	encodeJobs   domain.EncodingJobPublisher
	objects      domain.ObjectStorage
	playbacks    domain.PlaybackHistoryRepository
	uploadURLTTL time.Duration
	now          func() synchro.Time[tz.UTC]
}

func NewService(
	videos domain.VideoRepository,
	tags domain.VideoTagRepository,
	sharedKeys domain.SharedKeyRepository,
	deviceKeys domain.DeviceKeyRepository,
	uploads domain.UploadSessionRepository,
	progress domain.EncodingProgressRepository,
	encodeJobs domain.EncodingJobPublisher,
	objects domain.ObjectStorage,
	playbacks domain.PlaybackHistoryRepository,
	uploadURLTTL time.Duration,
) *Service {
	if uploadURLTTL <= 0 {
		uploadURLTTL = 15 * time.Minute
	}
	return &Service{
		videos:       videos,
		tags:         tags,
		sharedKeys:   sharedKeys,
		deviceKeys:   deviceKeys,
		uploads:      uploads,
		progress:     progress,
		encodeJobs:   encodeJobs,
		objects:      objects,
		playbacks:    playbacks,
		uploadURLTTL: uploadURLTTL,
		now:          synchro.Now[tz.UTC],
	}
}

func (s *Service) Me(ctx context.Context, userID string) (*domain.Me, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	keyVersions, err := s.sharedKeys.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	wrappedKeys, err := s.deviceKeys.ListWrappedSharedKeys(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &domain.Me{
		UserID:                  userID,
		SharedKeyVersions:       keyVersions,
		DeviceWrappedSharedKeys: wrappedKeys,
	}, nil
}

func (s *Service) RegisterSharedKey(ctx context.Context, userID string, publicKeyPEM string) (*domain.SharedKeyVersion, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if publicKeyPEM == "" {
		return nil, ErrInvalidInput
	}

	registered, err := s.sharedKeys.Register(ctx, userID, publicKeyPEM)
	if err != nil {
		return nil, err
	}

	return &registered, nil
}

func (s *Service) RevokeSharedKeyVersion(ctx context.Context, userID string, version int) (*domain.SharedKeyVersion, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	existing, err := s.sharedKeys.Get(ctx, userID, version)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.Status == domain.SharedKeyVersionStatusRevoked {
		return nil, ErrAlreadyRevoked
	}

	result, err := s.sharedKeys.Revoke(ctx, userID, version)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *Service) RegisterDeviceKey(
	ctx context.Context,
	userID string,
	deviceID string,
	devicePublicKeyPEM string,
	sharedKeyVersion int,
	encryptedSharedPrivateKey []byte,
) (*domain.DeviceWrappedSharedKey, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if deviceID == "" || devicePublicKeyPEM == "" || sharedKeyVersion <= 0 || len(encryptedSharedPrivateKey) == 0 {
		return nil, ErrInvalidInput
	}

	if _, err := s.sharedKeys.Get(ctx, userID, sharedKeyVersion); err != nil {
		return nil, err
	}

	registered, err := s.deviceKeys.RegisterWrappedSharedKey(ctx, userID, domain.DeviceWrappedSharedKey{
		DeviceID:                  deviceID,
		DevicePublicKeyPEM:        devicePublicKeyPEM,
		SharedKeyVersion:          sharedKeyVersion,
		EncryptedSharedPrivateKey: encryptedSharedPrivateKey,
		CreatedAt:                 s.now(),
	})
	if err != nil {
		return nil, err
	}

	return &registered, nil
}

func (s *Service) CreateUploadSession(ctx context.Context, userID string) (*domain.UploadSession, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	now := s.now()
	id := uuid.NewString()
	objectKey := fmt.Sprintf("videos/%s/%s/input", userID, id)
	session := domain.UploadSession{
		ID:        id,
		OwnerUser: userID,
		ObjectKey: objectKey,
		ExpiresAt: now.Add(s.uploadURLTTL),
		CreatedAt: now,
	}

	uploadURL, err := s.objects.CreateUploadURL(ctx, objectKey, s.uploadURLTTL)
	if err != nil {
		return nil, fmt.Errorf("create upload url: %w", err)
	}
	session.UploadURL = uploadURL

	if err := s.uploads.Create(ctx, session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *Service) CompleteUpload(ctx context.Context, userID string, uploadSessionID string) (*domain.Video, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	session, err := s.uploads.GetByID(ctx, uploadSessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.OwnerUser != userID || session.ExpiresAt.Before(s.now()) {
		return nil, ErrUploadSessionGone
	}

	exists, err := s.objects.Exists(ctx, session.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("check uploaded object: %w", err)
	}
	if !exists {
		return nil, ErrUploadObjectMissing
	}

	now := s.now()
	video := domain.Video{
		ID:              uuid.NewString(),
		OwnerUserID:     userID,
		Status:          domain.VideoStatusUploaded,
		SourceObjectKey: session.ObjectKey,
		UploadedAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.videos.Create(ctx, video); err != nil {
		return nil, err
	}

	if err := s.uploads.Delete(ctx, session.ID); err != nil {
		return nil, err
	}

	progress := domain.VideoEncodingProgress{
		VideoID:   video.ID,
		Status:    domain.VideoStatusUploaded,
		Percent:   0,
		UpdatedAt: now,
		OwnerUser: userID,
	}
	if err := s.progress.Set(ctx, progress); err != nil {
		return nil, err
	}

	if s.encodeJobs != nil {
		if err := s.encodeJobs.PublishEncodeVideo(ctx, domain.EncodeVideoJob{
			VideoID:         video.ID,
			OwnerUserID:     video.OwnerUserID,
			SourceObjectKey: video.SourceObjectKey,
		}); err != nil {
			return nil, fmt.Errorf("publish encode job: %w", err)
		}
	}

	return &video, nil
}

func (s *Service) UpdateVideoTags(
	ctx context.Context,
	userID string,
	videoID string,
	newTags []string,
) (*domain.Video, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, ErrNotFound
	}

	if err := s.tags.SetTags(ctx, videoID, newTags); err != nil {
		return nil, err
	}

	video.Tags = newTags
	return video, nil
}

func (s *Service) RetryEncoding(ctx context.Context, userID string, videoID string) (*domain.Video, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, ErrNotFound
	}

	if video.Status != domain.VideoStatusFailed {
		return nil, fmt.Errorf("%w: retry encoding requires FAILED status, got %s", ErrInvalidStateChange, video.Status)
	}

	// リトライジョブを投入するが、ステータスはFAILEDのまま維持する。
	// エンコーダがジョブをピックアップし、progressイベントを送信した時点でENCODINGに遷移する。
	// これにより、クライアントがポーリング中に一時的な不整合状態を観測することを防ぐ。
	video.FailedReason = nil
	video.UpdatedAt = s.now()

	if err := s.videos.Update(ctx, *video); err != nil {
		return nil, err
	}

	if s.encodeJobs != nil {
		if err := s.encodeJobs.PublishEncodeVideo(ctx, domain.EncodeVideoJob{
			VideoID:         video.ID,
			OwnerUserID:     video.OwnerUserID,
			SourceObjectKey: video.SourceObjectKey,
		}); err != nil {
			return nil, fmt.Errorf("publish encode job: %w", err)
		}
	}

	return video, nil
}

func (s *Service) ApplyEncodingEvent(ctx context.Context, event domain.EncodingEvent) error {
	video, err := s.videos.GetByID(ctx, event.VideoID)
	if err != nil {
		return err
	}
	if video == nil {
		return ErrNotFound
	}

	switch event.Type {
	case domain.EncodingEventTypeProgress:
		if event.Percent == nil {
			return ErrInvalidInput
		}
		message := event.Message
		return s.progress.Set(ctx, domain.VideoEncodingProgress{
			VideoID:   video.ID,
			Status:    domain.VideoStatusEncoding,
			Percent:   *event.Percent,
			UpdatedAt: s.now(),
			Message:   message,
			OwnerUser: video.OwnerUserID,
		})
	case domain.EncodingEventTypeCompleted:
		now := s.now()
		video.Status = domain.VideoStatusReady
		video.ReadyAt = &now
		video.FailedReason = nil
		video.DurationMillis = event.DurationMillis
		video.Width = event.Width
		video.Height = event.Height
		video.EncodedObjectKey = event.EncodedObjectKey
		video.UpdatedAt = now
		if err := s.videos.Update(ctx, *video); err != nil {
			return err
		}
		msg := "encoding completed"
		if event.Message != nil {
			msg = *event.Message
		}
		percent := 100.0
		return s.progress.Set(ctx, domain.VideoEncodingProgress{
			VideoID:   video.ID,
			Status:    domain.VideoStatusReady,
			Percent:   percent,
			UpdatedAt: s.now(),
			Message:   &msg,
			OwnerUser: video.OwnerUserID,
		})
	case domain.EncodingEventTypeFailed:
		now := s.now()
		video.Status = domain.VideoStatusFailed
		video.FailedReason = event.Message
		video.UpdatedAt = now
		if err := s.videos.Update(ctx, *video); err != nil {
			return err
		}
		percent := 100.0
		return s.progress.Set(ctx, domain.VideoEncodingProgress{
			VideoID:   video.ID,
			Status:    domain.VideoStatusFailed,
			Percent:   percent,
			UpdatedAt: s.now(),
			Message:   event.Message,
			OwnerUser: video.OwnerUserID,
		})
	default:
		return ErrInvalidInput
	}
}

func (s *Service) Video(ctx context.Context, userID string, videoID string) (*domain.Video, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, nil
	}

	tags, err := s.tags.GetTags(ctx, videoID)
	if err != nil {
		return nil, err
	}
	video.Tags = tags

	return video, nil
}

func (s *Service) Videos(ctx context.Context, userID string, status *domain.VideoStatus, tag *string, pagination domain.Pagination) (domain.VideoConnection, error) {
	if userID == "" {
		return domain.VideoConnection{}, ErrUnauthorized
	}
	if pagination.First <= 0 {
		pagination.First = 20
	}

	var conn domain.VideoConnection
	var err error
	if tag != nil {
		conn, err = s.videos.ListByOwnerAndTag(ctx, userID, *tag, status, pagination)
	} else {
		conn, err = s.videos.ListByOwner(ctx, userID, status, pagination)
	}
	if err != nil {
		return domain.VideoConnection{}, err
	}

	// バッチでタグを取得して各動画に設定する
	if len(conn.Edges) > 0 {
		videoIDs := make([]string, 0, len(conn.Edges))
		for _, e := range conn.Edges {
			videoIDs = append(videoIDs, e.Node.ID)
		}
		tagMap, err := s.tags.GetTagsBatch(ctx, videoIDs)
		if err != nil {
			return domain.VideoConnection{}, err
		}
		for i := range conn.Edges {
			conn.Edges[i].Node.Tags = tagMap[conn.Edges[i].Node.ID]
		}
	}

	return conn, nil
}

func (s *Service) EncodingProgress(ctx context.Context, userID string, videoID string) (*domain.VideoEncodingProgress, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, ErrNotFound
	}

	progress, err := s.progress.Get(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		defaultProgress := domain.VideoEncodingProgress{
			VideoID:   videoID,
			Status:    video.Status,
			Percent:   0,
			UpdatedAt: s.now(),
			OwnerUser: userID,
		}
		return &defaultProgress, nil
	}

	return progress, nil
}

func (s *Service) SubscribeEncodingProgress(
	ctx context.Context,
	userID string,
	videoID string,
) (<-chan domain.VideoEncodingProgress, func(), error) {
	if userID == "" {
		return nil, nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, nil, ErrNotFound
	}

	return s.progress.Subscribe(ctx, videoID)
}

func (s *Service) RateVideo(ctx context.Context, userID string, videoID string, rating *int) (*domain.Video, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, ErrNotFound
	}

	var domainRating *domain.Rating
	if rating != nil {
		r, err := domain.NewRating(*rating)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
		}
		domainRating = &r
	}

	if err := s.videos.UpdateRating(ctx, videoID, userID, domainRating); err != nil {
		return nil, err
	}

	video.Rating = domainRating

	tags, err := s.tags.GetTags(ctx, videoID)
	if err != nil {
		return nil, err
	}
	video.Tags = tags

	return video, nil
}

func (s *Service) RecordPlayback(ctx context.Context, userID string, videoID string) (*domain.PlaybackHistory, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	video, err := s.videos.GetByID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if video == nil || video.OwnerUserID != userID {
		return nil, ErrNotFound
	}

	now := s.now()
	history := domain.PlaybackHistory{
		ID:          uuid.NewString(),
		VideoID:     videoID,
		OwnerUserID: userID,
		PlayedAt:    now,
	}

	if err := s.playbacks.Record(ctx, history); err != nil {
		return nil, err
	}

	// 再生回数の非正規化フィールドを更新（失敗しても再生履歴の記録は維持する）
	if err := s.videos.IncrementPlayCount(ctx, videoID, now); err != nil {
		slog.ErrorContext(ctx, "再生回数の更新に失敗", slog.Any("error", err), slog.String("videoID", videoID))
	}

	return &history, nil
}
