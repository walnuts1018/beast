package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	pgxdriver "github.com/stephenafamo/bob/drivers/pgx"
	"github.com/stephenafamo/bob/types"
	"github.com/walnuts1018/beast/backend/internal/crypto"
	"github.com/walnuts1018/beast/backend/internal/db/generated/models"
	"github.com/walnuts1018/beast/backend/internal/domain"
)

type Postgres struct {
	db       pgxdriver.Pool
	mediaKey []byte
}

var _ Repository = (*Postgres)(nil)

func NewPostgres(ctx context.Context, databaseURL string, mediaKeys ...[]byte) (*Postgres, error) {
	db, err := pgxdriver.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	var mediaKey []byte
	if len(mediaKeys) > 0 {
		mediaKey = append([]byte(nil), mediaKeys[0]...)
	}
	return &Postgres{db: db, mediaKey: mediaKey}, nil
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

func (s *Postgres) CreateVideo(ctx context.Context, video domain.Video) (domain.Video, error) {
	videoID, err := uuid.Parse(video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	var tags, nonce []byte
	if len(s.mediaKey) == 32 {
		encodedTags, marshalErr := json.Marshal(video.Tags)
		if marshalErr != nil {
			return domain.Video{}, marshalErr
		}
		tags, nonce, err = crypto.EncryptTags(encodedTags, s.mediaKey)
	} else if video.TagsCiphertext != "" {
		tags, err = base64.RawStdEncoding.DecodeString(video.TagsCiphertext)
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("encrypt tags: %w", err)
	}
	dataKey, err := base64.RawStdEncoding.DecodeString(video.Encryption.EncryptedDataKey)
	if err != nil {
		return domain.Video{}, fmt.Errorf("decode encrypted data key: %w", err)
	}
	status := string(video.Status)
	chunkSize := normalizedChunkSize(video.Encryption.ChunkSize)
	sourceObjectKey := video.SourceObjectKey
	if sourceObjectKey == "" {
		sourceObjectKey = video.ObjectKey
	}
	progress := video.Progress
	errorMessage := video.ErrorMessage
	artifacts, err := marshalArtifacts(video.HLSArtifacts)
	if err != nil {
		return domain.Video{}, err
	}
	hlsArtifacts := types.NewJSON(json.RawMessage(artifacts))
	keyVersion := "media-v1"
	row, err := models.Videos.Insert(&models.VideoSetter{ID: &videoID, OwnerID: &video.OwnerID, Status: &status, ObjectKey: &video.ObjectKey, SourceObjectKey: &sourceObjectKey, TagsCiphertext: &tags, TagsNonce: &nonce, EncryptedDataKey: &dataKey, EncryptionAlgorithm: &video.Encryption.Algorithm, ChunkSize: &chunkSize, EncryptionKeyVersion: &keyVersion, Progress: &progress, ErrorMessage: &errorMessage, HLSArtifacts: &hlsArtifacts}).One(ctx, s.db)
	if err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row, s.mediaKey), nil
}

func (s *Postgres) UpdateVideo(ctx context.Context, video domain.Video) (domain.Video, error) {
	row, err := s.GetVideoRow(ctx, video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	status := string(video.Status)
	objectKey := video.ObjectKey
	sourceObjectKey := video.SourceObjectKey
	progress := video.Progress
	errorMessage := video.ErrorMessage
	chunkSize := normalizedChunkSize(video.Encryption.ChunkSize)
	algorithm := video.Encryption.Algorithm
	keyVersion := "media-v1"
	dataKey, err := base64.RawStdEncoding.DecodeString(video.Encryption.EncryptedDataKey)
	if err != nil {
		return domain.Video{}, fmt.Errorf("decode encrypted data key: %w", err)
	}
	artifacts, err := marshalArtifacts(video.HLSArtifacts)
	if err != nil {
		return domain.Video{}, err
	}
	hlsArtifacts := types.NewJSON(json.RawMessage(artifacts))
	var rating sql.Null[int16]
	if video.Rating != nil {
		rating = sql.Null[int16]{V: int16(*video.Rating), Valid: true}
	}
	var lastPlayedAt sql.Null[time.Time]
	if video.LastPlayedAt != nil {
		lastPlayedAt = sql.Null[time.Time]{V: *video.LastPlayedAt, Valid: true}
	}
	updatedAt := video.UpdatedAt
	if err := row.Update(ctx, s.db, &models.VideoSetter{Status: &status, ObjectKey: &objectKey, SourceObjectKey: &sourceObjectKey, EncryptedDataKey: &dataKey, EncryptionAlgorithm: &algorithm, ChunkSize: &chunkSize, EncryptionKeyVersion: &keyVersion, PlayCount: &video.PlayCount, Rating: &rating, LastPlayedAt: &lastPlayedAt, UpdatedAt: &updatedAt, Progress: &progress, ErrorMessage: &errorMessage, HLSArtifacts: &hlsArtifacts}); err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row, s.mediaKey), nil
}

func (s *Postgres) ListVideos(ctx context.Context, ownerID string) ([]domain.Video, error) {
	rows, err := models.Videos.Query(models.SelectWhere.Videos.OwnerID.EQ(ownerID), sm.OrderBy(models.Videos.Columns.CreatedAt).Desc()).All(ctx, s.db)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Video, 0, len(rows))
	for _, row := range rows {
		result = append(result, videoFromDB(row, s.mediaKey))
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
	return videoFromDB(row, s.mediaKey), nil
}

func (s *Postgres) GetVideoByObjectKey(ctx context.Context, ownerID, objectKey string) (domain.Video, error) {
	row, err := models.Videos.Query(models.SelectWhere.Videos.ObjectKey.EQ(objectKey), models.SelectWhere.Videos.OwnerID.EQ(ownerID)).One(ctx, s.db)
	if err != nil {
		return domain.Video{}, err
	}
	return videoFromDB(row, s.mediaKey), nil
}

func (s *Postgres) UpdateVideoTags(ctx context.Context, ownerID, id string, tagsInput []string) (domain.Video, error) {
	video, err := s.GetVideo(ctx, ownerID, id)
	if err != nil {
		return domain.Video{}, err
	}
	row, err := s.GetVideoRow(ctx, video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	updatedAt := time.Now().UTC()
	var tags []byte
	var nonce []byte
	normalizedTags, err := domain.NormalizeTags(tagsInput)
	if err != nil {
		return domain.Video{}, err
	}
	if len(s.mediaKey) != 32 {
		return domain.Video{}, errors.New("media encryption key is unavailable")
	}
	encoded, marshalErr := json.Marshal(normalizedTags)
	if marshalErr != nil {
		return domain.Video{}, marshalErr
	}
	tags, nonce, err = crypto.EncryptTags(encoded, s.mediaKey)
	if err != nil {
		return domain.Video{}, err
	}
	if err := row.Update(ctx, s.db, &models.VideoSetter{TagsCiphertext: &tags, TagsNonce: &nonce, UpdatedAt: &updatedAt}); err != nil {
		return domain.Video{}, err
	}
	video.TagsCiphertext = base64.RawStdEncoding.EncodeToString(tags)
	video.TagsNonce = base64.RawStdEncoding.EncodeToString(nonce)
	if normalizedTags != nil {
		video.Tags = normalizedTags
	}
	video.UpdatedAt = updatedAt
	return video, nil
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
	return videoFromDB(row, s.mediaKey), nil
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
	return videoFromDB(row, s.mediaKey), nil
}

func (s *Postgres) GetVideoRow(ctx context.Context, id string) (*models.Video, error) {
	videoID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return models.Videos.Query(models.SelectWhere.Videos.ID.EQ(videoID)).One(ctx, s.db)
}

func videoFromDB(row *models.Video, mediaKeys ...[]byte) domain.Video {
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
	artifacts := make(map[string]domain.HLSArtifact)
	if len(row.HLSArtifacts.Val) > 0 {
		_ = json.Unmarshal(row.HLSArtifacts.Val, &artifacts)
	}
	video := domain.Video{ID: row.ID.String(), OwnerID: row.OwnerID, Status: domain.VideoStatus(row.Status), ObjectKey: row.ObjectKey, SourceObjectKey: row.SourceObjectKey, TagsCiphertext: base64.RawStdEncoding.EncodeToString(row.TagsCiphertext), TagsNonce: base64.RawStdEncoding.EncodeToString(row.TagsNonce), PlayCount: row.PlayCount, Rating: rating, LastPlayedAt: lastPlayedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Progress: row.Progress, ErrorMessage: row.ErrorMessage, HLSArtifacts: artifacts, Encryption: domain.EncryptionMetadata{Algorithm: row.EncryptionAlgorithm, ChunkSize: int(row.ChunkSize), EncryptedDataKey: base64.RawStdEncoding.EncodeToString(row.EncryptedDataKey)}}
	if manifest, ok := artifacts["manifest.m3u8"]; ok {
		video.Encryption = manifest.Encryption
	} else if manifest, ok := artifacts["manifest.mpd"]; ok {
		video.Encryption = manifest.Encryption
	}
	if len(mediaKeys) > 0 && len(mediaKeys[0]) == 32 {
		if plaintext, err := crypto.DecryptTags(row.TagsCiphertext, row.TagsNonce, mediaKeys[0]); err == nil {
			_ = json.Unmarshal(plaintext, &video.Tags)
		}
	}
	return video
}

func marshalArtifacts(artifacts map[string]domain.HLSArtifact) ([]byte, error) {
	if artifacts == nil {
		artifacts = map[string]domain.HLSArtifact{}
	}
	data, err := json.Marshal(artifacts)
	if err != nil {
		return nil, fmt.Errorf("marshal HLS artifacts: %w", err)
	}
	return data, nil
}

func normalizedChunkSize(chunkSize int) int32 {
	if chunkSize <= 0 {
		return crypto.AtRestChunkSize
	}
	return int32(chunkSize)
}
