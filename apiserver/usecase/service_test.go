package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/walnuts1018/beast/apiserver/domain"
	"github.com/walnuts1018/beast/apiserver/infra/memory"
	"github.com/walnuts1018/beast/apiserver/usecase"
)

// stubObjectStorage はテスト用のオブジェクトストレージ
type stubObjectStorage struct {
	objects map[string]bool
}

func newStubObjectStorage() *stubObjectStorage {
	return &stubObjectStorage{objects: make(map[string]bool)}
}

func (s *stubObjectStorage) CreateUploadURL(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return fmt.Sprintf("https://fake-s3/%s", objectKey), nil
}

func (s *stubObjectStorage) Exists(_ context.Context, objectKey string) (bool, error) {
	return s.objects[objectKey], nil
}

func (s *stubObjectStorage) HealthCheck(_ context.Context) error {
	return nil
}

func newTestService() (*usecase.Service, *memory.Store, *stubObjectStorage) {
	store := memory.NewStore()
	objects := newStubObjectStorage()
	svc := usecase.NewService(
		memory.NewVideoRepository(store),
		memory.NewSharedKeyRepository(store),
		memory.NewDeviceKeyRepository(store),
		memory.NewUploadSessionRepository(store),
		memory.NewEncodingProgressRepository(store),
		objects,
	)
	return svc, store, objects
}

func TestMe_EmptyUserID(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Me(context.Background(), "")
	if !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestMe_Success(t *testing.T) {
	svc, _, _ := newTestService()
	me, err := svc.Me(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if me.UserID != "user-1" {
		t.Fatalf("expected user-1, got %s", me.UserID)
	}
	if len(me.SharedKeyVersions) != 0 {
		t.Fatalf("expected 0 shared keys, got %d", len(me.SharedKeyVersions))
	}
	if len(me.DeviceWrappedSharedKeys) != 0 {
		t.Fatalf("expected 0 device keys, got %d", len(me.DeviceWrappedSharedKeys))
	}
}

func TestRegisterSharedKey_EmptyUserID(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.RegisterSharedKey(context.Background(), "", "pem")
	if !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestRegisterSharedKey_EmptyPEM(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.RegisterSharedKey(context.Background(), "user-1", "")
	if !errors.Is(err, usecase.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRegisterSharedKey_Success(t *testing.T) {
	svc, _, _ := newTestService()
	v, err := svc.RegisterSharedKey(context.Background(), "user-1", "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Version != 1 {
		t.Fatalf("expected version 1, got %d", v.Version)
	}
	if v.Status != domain.SharedKeyVersionStatusActive {
		t.Fatalf("expected ACTIVE, got %s", v.Status)
	}
}

func TestRegisterSharedKey_VersionIncrement(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	v1, _ := svc.RegisterSharedKey(ctx, "user-1", "pem-1")
	v2, _ := svc.RegisterSharedKey(ctx, "user-1", "pem-2")

	if v1.Version != 1 || v2.Version != 2 {
		t.Fatalf("expected version 1 and 2, got %d and %d", v1.Version, v2.Version)
	}
}

func TestRevokeSharedKeyVersion_NotFound(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.RevokeSharedKeyVersion(context.Background(), "user-1", 999)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestRevokeSharedKeyVersion_AlreadyRevoked(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	svc.RegisterSharedKey(ctx, "user-1", "pem-1")
	svc.RevokeSharedKeyVersion(ctx, "user-1", 1)
	_, err := svc.RevokeSharedKeyVersion(ctx, "user-1", 1)

	if !errors.Is(err, usecase.ErrAlreadyRevoked) {
		t.Fatalf("expected ErrAlreadyRevoked, got %v", err)
	}
}

func TestRevokeSharedKeyVersion_Success(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	svc.RegisterSharedKey(ctx, "user-1", "pem-1")
	v, err := svc.RevokeSharedKeyVersion(ctx, "user-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Status != domain.SharedKeyVersionStatusRevoked {
		t.Fatalf("expected REVOKED, got %s", v.Status)
	}
	if v.RevokedAt == nil {
		t.Fatalf("expected RevokedAt to be set")
	}
}

func TestRegisterDeviceKey_InvalidInput(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	tests := []struct {
		name     string
		deviceID string
		pem      string
		version  int
		data     []byte
	}{
		{"empty deviceID", "", "pem", 1, []byte("data")},
		{"empty pem", "device-1", "", 1, []byte("data")},
		{"zero version", "device-1", "pem", 0, []byte("data")},
		{"empty data", "device-1", "pem", 1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.RegisterDeviceKey(ctx, "user-1", tt.deviceID, tt.pem, tt.version, tt.data)
			if !errors.Is(err, usecase.ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestRegisterDeviceKey_Success(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	svc.RegisterSharedKey(ctx, "user-1", "pem-1")

	result, err := svc.RegisterDeviceKey(ctx, "user-1", "device-1", "device-pem", 1, []byte("encrypted-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DeviceID != "device-1" {
		t.Fatalf("expected device-1, got %s", result.DeviceID)
	}
	if result.DevicePublicKeyPEM != "device-pem" {
		t.Fatalf("expected device-pem, got %s", result.DevicePublicKeyPEM)
	}
}

func TestRegisterDeviceKey_Upsert(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	svc.RegisterSharedKey(ctx, "user-1", "pem-1")
	svc.RegisterDeviceKey(ctx, "user-1", "device-1", "pem-old", 1, []byte("data-old"))
	svc.RegisterDeviceKey(ctx, "user-1", "device-1", "pem-new", 1, []byte("data-new"))

	me, _ := svc.Me(ctx, "user-1")
	if len(me.DeviceWrappedSharedKeys) != 1 {
		t.Fatalf("expected 1 device key (upsert), got %d", len(me.DeviceWrappedSharedKeys))
	}
	if string(me.DeviceWrappedSharedKeys[0].EncryptedSharedPrivateKey) != "data-new" {
		t.Fatalf("expected updated data")
	}
}

func TestCreateUploadSession_EmptyUserID(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.CreateUploadSession(context.Background(), "")
	if !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestCreateUploadSession_Success(t *testing.T) {
	svc, _, _ := newTestService()
	session, err := svc.CreateUploadSession(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
	if session.UploadURL == "" {
		t.Fatalf("expected non-empty upload URL")
	}
	if session.OwnerUser != "user-1" {
		t.Fatalf("expected user-1, got %s", session.OwnerUser)
	}
}

func TestCompleteUpload_SessionNotFound(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.CompleteUpload(context.Background(), "user-1", "non-existent")
	if !errors.Is(err, usecase.ErrUploadSessionGone) {
		t.Fatalf("expected ErrUploadSessionGone, got %v", err)
	}
}

func TestCompleteUpload_ObjectMissing(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	// オブジェクトをアップロードしていないため、CompleteUploadは失敗するはず
	_, err := svc.CompleteUpload(ctx, "user-1", session.ID)
	if !errors.Is(err, usecase.ErrUploadObjectMissing) {
		t.Fatalf("expected ErrUploadObjectMissing, got %v", err)
	}
}

func TestCompleteUpload_Success(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	// オブジェクトが存在することをシミュレート
	objects.objects[session.ObjectKey] = true

	video, err := svc.CompleteUpload(ctx, "user-1", session.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if video.ID == "" {
		t.Fatalf("expected non-empty video ID")
	}
	if video.Status != domain.VideoStatusUploaded {
		t.Fatalf("expected UPLOADED, got %s", video.Status)
	}
	if video.OwnerUserID != "user-1" {
		t.Fatalf("expected user-1, got %s", video.OwnerUserID)
	}
}

func TestCompleteUpload_WrongUser(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true

	_, err := svc.CompleteUpload(ctx, "user-2", session.ID)
	if !errors.Is(err, usecase.ErrUploadSessionGone) {
		t.Fatalf("expected ErrUploadSessionGone, got %v", err)
	}
}

func TestRetryEncoding_NotFailed(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	// UPLOADED状態のビデオをリトライしようとする
	_, err := svc.RetryEncoding(ctx, "user-1", video.ID)
	if !errors.Is(err, usecase.ErrInvalidStateChange) {
		t.Fatalf("expected ErrInvalidStateChange, got %v", err)
	}
}

func TestRetryEncoding_WrongUser(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	_, err := svc.RetryEncoding(ctx, "user-2", video.ID)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateEncryptedTags_Success(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	tags := []byte("encrypted-tags")
	enc := domain.EncryptionMetadata{
		Algorithm:        domain.EncryptionAlgorithmXChaCha20Poly1305,
		KeyVersion:       1,
		Nonce:            []byte("nonce"),
		EncryptedDataKey: []byte("data-key"),
	}

	updated, err := svc.UpdateEncryptedTags(ctx, "user-1", video.ID, tags, enc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(updated.EncryptedTags) != "encrypted-tags" {
		t.Fatalf("expected encrypted-tags, got %s", string(updated.EncryptedTags))
	}
}

func TestUpdateEncryptedTags_WrongUser(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	_, err := svc.UpdateEncryptedTags(ctx, "user-2", video.ID, []byte("tags"), domain.EncryptionMetadata{})
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestVideo_WrongUser(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	result, err := svc.Video(ctx, "user-2", video.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for wrong user, got video")
	}
}

func TestVideo_Success(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	result, err := svc.Video(ctx, "user-1", video.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != video.ID {
		t.Fatalf("expected %s, got %s", video.ID, result.ID)
	}
}

func TestVideos_EmptyUserID(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.Videos(context.Background(), "", nil, domain.Pagination{})
	if !errors.Is(err, usecase.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestVideos_Pagination(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	// 3本のビデオを作成
	for range 3 {
		session, _ := svc.CreateUploadSession(ctx, "user-1")
		objects.objects[session.ObjectKey] = true
		svc.CompleteUpload(ctx, "user-1", session.ID)
	}

	conn, err := svc.Videos(ctx, "user-1", nil, domain.Pagination{First: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conn.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(conn.Edges))
	}
	if !conn.HasNext {
		t.Fatalf("expected hasNext to be true")
	}
}

func TestEncodingProgress_DefaultProgress(t *testing.T) {
	svc, _, objects := newTestService()
	ctx := context.Background()

	session, _ := svc.CreateUploadSession(ctx, "user-1")
	objects.objects[session.ObjectKey] = true
	video, _ := svc.CompleteUpload(ctx, "user-1", session.ID)

	progress, err := svc.EncodingProgress(ctx, "user-1", video.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if progress.VideoID != video.ID {
		t.Fatalf("expected %s, got %s", video.ID, progress.VideoID)
	}
	if progress.Status != domain.VideoStatusUploaded {
		t.Fatalf("expected UPLOADED, got %s", progress.Status)
	}
}

func TestMeWithKeys_AfterRegistration(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	svc.RegisterSharedKey(ctx, "user-1", "pem-1")
	svc.RegisterSharedKey(ctx, "user-1", "pem-2")
	svc.RegisterDeviceKey(ctx, "user-1", "device-1", "device-pem-1", 1, []byte("encrypted"))

	me, err := svc.Me(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(me.SharedKeyVersions) != 2 {
		t.Fatalf("expected 2 shared keys, got %d", len(me.SharedKeyVersions))
	}
	if len(me.DeviceWrappedSharedKeys) != 1 {
		t.Fatalf("expected 1 device key, got %d", len(me.DeviceWrappedSharedKeys))
	}
}
