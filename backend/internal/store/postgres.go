package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	pgxdriver "github.com/stephenafamo/bob/drivers/pgx"
	"github.com/walnuts1018/beast/backend/internal/db/generated/models"
	"github.com/walnuts1018/beast/backend/internal/domain"
)

type Postgres struct {
	db pgxdriver.Pool
}

var _ Repository = (*Postgres)(nil)

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	db, err := pgxdriver.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (s *Postgres) Close() { s.db.Close() }

func (s *Postgres) Migrate(ctx context.Context, schema string) error {
	if schema == "" {
		return errors.New("schema is required")
	}
	if _, err := s.db.Exec(ctx, schema); err != nil {
		return fmt.Errorf("apply database schema: %w", err)
	}
	return nil
}

func (s *Postgres) RegisterSharedKey(ctx context.Context, key SharedKey) (SharedKey, error) {
	id, err := uuid.Parse(key.ID)
	if err != nil {
		return SharedKey{}, fmt.Errorf("parse shared key ID: %w", err)
	}
	row, err := models.SharedKeys.Insert(&models.SharedKeySetter{ID: &id, OwnerID: &key.OwnerID, Version: &key.Version, PublicKey: &key.PublicKey, Status: &key.Status}).One(ctx, s.db)
	if err != nil {
		return SharedKey{}, err
	}
	return sharedKeyFromDB(row), nil
}

func (s *Postgres) SharedKey(ctx context.Context, ownerID, id string) (SharedKey, error) {
	keyID, err := uuid.Parse(id)
	if err != nil {
		return SharedKey{}, err
	}
	row, err := models.SharedKeys.Query(models.SelectWhere.SharedKeys.ID.EQ(keyID), models.SelectWhere.SharedKeys.OwnerID.EQ(ownerID)).One(ctx, s.db)
	if err != nil {
		return SharedKey{}, err
	}
	return sharedKeyFromDB(row), nil
}

func (s *Postgres) ListSharedKeys(ctx context.Context, ownerID string) ([]SharedKey, error) {
	rows, err := models.SharedKeys.Query(models.SelectWhere.SharedKeys.OwnerID.EQ(ownerID), sm.OrderBy(models.SharedKeys.Columns.CreatedAt).Desc()).All(ctx, s.db)
	if err != nil {
		return nil, err
	}
	result := make([]SharedKey, 0, len(rows))
	for _, row := range rows {
		result = append(result, sharedKeyFromDB(row))
	}
	return result, nil
}

func (s *Postgres) RegisterDeviceKey(ctx context.Context, key DeviceKey) (DeviceKey, error) {
	id, err := uuid.Parse(key.ID)
	if err != nil {
		return DeviceKey{}, err
	}
	sharedKeyID, err := uuid.Parse(key.SharedKeyID)
	if err != nil {
		return DeviceKey{}, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(key.EncryptedSharedPrivateKey)
	if err != nil {
		return DeviceKey{}, fmt.Errorf("decode encrypted device key: %w", err)
	}
	row, err := models.DeviceKeys.Insert(&models.DeviceKeySetter{ID: &id, OwnerID: &key.OwnerID, DeviceID: &key.DeviceID, SharedKeyID: &sharedKeyID, EncryptedSharedPrivateKey: &ciphertext}).One(ctx, s.db)
	if err != nil {
		return DeviceKey{}, err
	}
	return deviceKeyFromDB(row), nil
}

func (s *Postgres) ListDeviceKeys(ctx context.Context, ownerID string) ([]DeviceKey, error) {
	rows, err := models.DeviceKeys.Query(models.SelectWhere.DeviceKeys.OwnerID.EQ(ownerID), sm.OrderBy(models.DeviceKeys.Columns.CreatedAt).Desc()).All(ctx, s.db)
	if err != nil {
		return nil, err
	}
	result := make([]DeviceKey, 0, len(rows))
	for _, row := range rows {
		result = append(result, deviceKeyFromDB(row))
	}
	return result, nil
}

func (s *Postgres) CreateVideo(ctx context.Context, video domain.Video) (domain.Video, error) {
	videoID, err := uuid.Parse(video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	sharedKeyID, err := uuid.Parse(video.Encryption.SharedKeyID)
	if err != nil {
		return domain.Video{}, err
	}
	tags, err := base64.RawStdEncoding.DecodeString(video.EncryptedTags)
	if err != nil {
		return domain.Video{}, fmt.Errorf("decode encrypted tags: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(video.Encryption.Nonce)
	if err != nil {
		return domain.Video{}, fmt.Errorf("decode encryption nonce: %w", err)
	}
	dataKey, err := base64.RawStdEncoding.DecodeString(video.Encryption.EncryptedDataKey)
	if err != nil {
		return domain.Video{}, fmt.Errorf("decode encrypted data key: %w", err)
	}
	status := string(video.Status)
	chunkSize := int32(video.Encryption.ChunkSize)
	row, err := models.Videos.Insert(&models.VideoSetter{ID: &videoID, OwnerID: &video.OwnerID, Status: &status, ObjectKey: &video.ObjectKey, TagsCiphertext: &tags, TagsNonce: &nonce, EncryptedDataKey: &dataKey, EncryptionAlgorithm: &video.Encryption.Algorithm, ChunkSize: &chunkSize, EncryptionKeyVersion: &video.Encryption.KeyVersion, SharedKeyID: &sharedKeyID}).One(ctx, s.db)
	if err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row), nil
}

func (s *Postgres) ListVideos(ctx context.Context, ownerID string) ([]domain.Video, error) {
	rows, err := models.Videos.Query(models.SelectWhere.Videos.OwnerID.EQ(ownerID), sm.OrderBy(models.Videos.Columns.CreatedAt).Desc()).All(ctx, s.db)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Video, 0, len(rows))
	for _, row := range rows {
		result = append(result, videoFromDB(row))
	}
	return result, nil
}

func (s *Postgres) GetVideo(ctx context.Context, ownerID, id string) (domain.Video, error) {
	videoID, err := uuid.Parse(id)
	if err != nil {
		return domain.Video{}, err
	}
	row, err := models.Videos.Query(models.SelectWhere.Videos.ID.EQ(videoID), models.SelectWhere.Videos.OwnerID.EQ(ownerID)).One(ctx, s.db)
	if err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row), nil
}

func (s *Postgres) RecordPlayback(ctx context.Context, ownerID, id string) (domain.Video, error) {
	video, err := s.GetVideo(ctx, ownerID, id)
	if err != nil {
		return domain.Video{}, err
	}
	video.RecordPlayback(time.Now())
	row, err := s.GetVideoRow(ctx, video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	playCount := video.PlayCount
	lastPlayedAt := sql.Null[time.Time]{V: *video.LastPlayedAt, Valid: true}
	updatedAt := video.UpdatedAt
	if err := row.Update(ctx, s.db, &models.VideoSetter{PlayCount: &playCount, LastPlayedAt: &lastPlayedAt, UpdatedAt: &updatedAt}); err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row), nil
}

func (s *Postgres) SetRating(ctx context.Context, ownerID, id string, rating *int) (domain.Video, error) {
	video, err := s.GetVideo(ctx, ownerID, id)
	if err != nil {
		return domain.Video{}, err
	}
	if err := video.SetRating(rating); err != nil {
		return domain.Video{}, err
	}
	row, err := s.GetVideoRow(ctx, video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	var value sql.Null[int16]
	if rating != nil {
		value = sql.Null[int16]{V: int16(*rating), Valid: true}
	}
	updatedAt := video.UpdatedAt
	if err := row.Update(ctx, s.db, &models.VideoSetter{Rating: &value, UpdatedAt: &updatedAt}); err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row), nil
}

func (s *Postgres) GetVideoRow(ctx context.Context, id string) (*models.Video, error) {
	videoID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return models.Videos.Query(models.SelectWhere.Videos.ID.EQ(videoID)).One(ctx, s.db)
}

func sharedKeyFromDB(row *models.SharedKey) SharedKey {
	return SharedKey{ID: row.ID.String(), OwnerID: row.OwnerID, Version: row.Version, PublicKey: row.PublicKey, Status: row.Status}
}

func deviceKeyFromDB(row *models.DeviceKey) DeviceKey {
	return DeviceKey{ID: row.ID.String(), OwnerID: row.OwnerID, DeviceID: row.DeviceID, SharedKeyID: row.SharedKeyID.String(), EncryptedSharedPrivateKey: base64.RawStdEncoding.EncodeToString(row.EncryptedSharedPrivateKey)}
}

func videoFromDB(row *models.Video) domain.Video {
	var rating *int
	if row.Rating.Valid {
		ratingValue := int(row.Rating.V)
		rating = &ratingValue
	}
	var lastPlayedAt *time.Time
	if row.LastPlayedAt.Valid {
		lastPlayedAtValue := row.LastPlayedAt.V
		lastPlayedAt = &lastPlayedAtValue
	}
	return domain.Video{ID: row.ID.String(), OwnerID: row.OwnerID, Status: domain.VideoStatus(row.Status), ObjectKey: row.ObjectKey, EncryptedTags: base64.RawStdEncoding.EncodeToString(row.TagsCiphertext), PlayCount: row.PlayCount, Rating: rating, LastPlayedAt: lastPlayedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Encryption: domain.EncryptionMetadata{Algorithm: row.EncryptionAlgorithm, ChunkSize: int(row.ChunkSize), KeyVersion: row.EncryptionKeyVersion, Nonce: base64.RawStdEncoding.EncodeToString(row.TagsNonce), EncryptedDataKey: base64.RawStdEncoding.EncodeToString(row.EncryptedDataKey), SharedKeyID: row.SharedKeyID.String()}}
}
