package domain

import (
	"errors"
	"time"

	"uuid"
)

var (
	ErrVideoNotFound = errors.New("video not found")
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	ErrInvalidStatus = errors.New("invalid video status")
)

type VideoStatus string

const (
	VideoStatusUploaded VideoStatus = "UPLOADED"
	VideoStatusEncoding VideoStatus = "ENCODING"
	VideoStatusReady    VideoStatus = "READY"
	VideoStatusFailed   VideoStatus = "FAILED"
)

type EncryptionMetadata struct {
	Algorithm        string
	ChunkSize        int
	Nonce            string
	EncryptedDataKey string
	PlaintextSize    int64
}

type HLSArtifact struct {
	ObjectKey  string
	Encryption EncryptionMetadata
}

func (v *Video) TransitionStatus(next VideoStatus) error {
	valid := map[VideoStatus]bool{VideoStatusUploaded: true, VideoStatusEncoding: true, VideoStatusReady: true, VideoStatusFailed: true}
	if !valid[next] {
		return ErrInvalidStatus
	}
	allowed := map[VideoStatus]map[VideoStatus]bool{
		VideoStatusUploaded: {VideoStatusEncoding: true},
		VideoStatusEncoding: {VideoStatusReady: true, VideoStatusFailed: true},
		VideoStatusFailed:   {VideoStatusEncoding: true},
		VideoStatusReady:    {},
	}
	if !allowed[v.Status][next] {
		return ErrInvalidStatus
	}
	v.Status = next
	v.UpdatedAt = time.Now().UTC()
	return nil
}

type Video struct {
	ID              string
	OwnerID         string
	Status          VideoStatus
	ObjectKey       string
	SourceObjectKey string
	TagsCiphertext  string
	TagsNonce       string
	Tags            []string
	PlayCount       int64
	Rating          *int
	LastPlayedAt    *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Encryption      EncryptionMetadata
	Progress        float64
	ErrorMessage    string
	HLSArtifacts    map[string]HLSArtifact
}

func NewServerVideo(ownerID string, tags []string, objectKey string) (Video, error) {
	if ownerID == "" || objectKey == "" {
		return Video{}, errors.New("owner and object key are required")
	}
	tags, err := NormalizeTags(tags)
	if err != nil {
		return Video{}, err
	}
	now := time.Now().UTC()
	return Video{ID: uuid.New().String(), OwnerID: ownerID, Status: VideoStatusUploaded, ObjectKey: objectKey, SourceObjectKey: objectKey, Tags: append([]string(nil), tags...), CreatedAt: now, UpdatedAt: now, HLSArtifacts: make(map[string]HLSArtifact)}, nil
}

func (v *Video) SetProgress(progress float64) error {
	if progress < 0 || progress > 1 {
		return errors.New("progress must be between 0 and 1")
	}
	v.Progress = progress
	v.UpdatedAt = time.Now().UTC()
	return nil
}

func (v *Video) SetRating(rating *int) error {
	if rating != nil && (*rating < 1 || *rating > 5) {
		return ErrInvalidRating
	}
	v.Rating = rating
	v.UpdatedAt = time.Now().UTC()
	return nil
}

func (v *Video) RecordPlayback(now time.Time) {
	v.PlayCount++
	t := now.UTC()
	v.LastPlayedAt = &t
	v.UpdatedAt = t
}

func (v *Video) SetStatus(status VideoStatus) error {
	return v.TransitionStatus(status)
}
