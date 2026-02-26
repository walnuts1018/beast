package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	videos     domain.VideoRepository
	sharedKeys domain.SharedKeyRepository
	deviceKeys domain.DeviceKeyRepository
	uploads    domain.UploadSessionRepository
	progress   domain.EncodingProgressRepository
	objects    domain.ObjectStorage
	now        func() time.Time
}

func NewService(
	videos domain.VideoRepository,
	sharedKeys domain.SharedKeyRepository,
	deviceKeys domain.DeviceKeyRepository,
	uploads domain.UploadSessionRepository,
	progress domain.EncodingProgressRepository,
	objects domain.ObjectStorage,
) *Service {
	return &Service{
		videos:     videos,
		sharedKeys: sharedKeys,
		deviceKeys: deviceKeys,
		uploads:    uploads,
		progress:   progress,
		objects:    objects,
		now:        time.Now,
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
		CreatedAt:                 s.now().UTC(),
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

	now := s.now().UTC()
	id := uuid.NewString()
	objectKey := fmt.Sprintf("videos/%s/%s/input", userID, id)
	session := domain.UploadSession{
		ID:        id,
		OwnerUser: userID,
		ObjectKey: objectKey,
		ExpiresAt: now.Add(15 * time.Minute),
		CreatedAt: now,
	}

	uploadURL, err := s.objects.CreateUploadURL(ctx, objectKey, 15*time.Minute)
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
	if session == nil || session.OwnerUser != userID || session.ExpiresAt.Before(s.now().UTC()) {
		return nil, ErrUploadSessionGone
	}

	exists, err := s.objects.Exists(ctx, session.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("check uploaded object: %w", err)
	}
	if !exists {
		return nil, ErrUploadObjectMissing
	}

	now := s.now().UTC()
	video := domain.Video{
		ID:            uuid.NewString(),
		OwnerUserID:   userID,
		Status:        domain.VideoStatusUploaded,
		UploadedAt:    now,
		EncryptedTags: []byte{},
		TagEncryption: domain.EncryptionMetadata{
			Algorithm:        domain.EncryptionAlgorithmXChaCha20Poly1305,
			KeyVersion:       1,
			Nonce:            []byte{},
			EncryptedDataKey: []byte{},
		},
		CreatedAt: now,
		UpdatedAt: now,
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

	return &video, nil
}

func (s *Service) UpdateEncryptedTags(
	ctx context.Context,
	userID string,
	videoID string,
	encryptedTags []byte,
	tagEncryption domain.EncryptionMetadata,
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

	video.EncryptedTags = encryptedTags
	video.TagEncryption = tagEncryption
	video.UpdatedAt = s.now().UTC()

	if err := s.videos.Update(ctx, *video); err != nil {
		return nil, err
	}

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

	video.Status = domain.VideoStatusEncoding
	video.FailedReason = nil
	video.UpdatedAt = s.now().UTC()

	if err := s.videos.Update(ctx, *video); err != nil {
		return nil, err
	}

	progress := domain.VideoEncodingProgress{
		VideoID:   video.ID,
		Status:    domain.VideoStatusEncoding,
		Percent:   0,
		UpdatedAt: s.now().UTC(),
		OwnerUser: userID,
	}

	if err := s.progress.Set(ctx, progress); err != nil {
		return nil, err
	}

	return video, nil
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

	return video, nil
}

func (s *Service) Videos(ctx context.Context, userID string, status *domain.VideoStatus, pagination domain.Pagination) (domain.VideoConnection, error) {
	if userID == "" {
		return domain.VideoConnection{}, ErrUnauthorized
	}
	if pagination.First <= 0 {
		pagination.First = 20
	}

	return s.videos.ListByOwner(ctx, userID, status, pagination)
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
			UpdatedAt: s.now().UTC(),
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
