package postgres

import (
	"github.com/Code-Hex/synchro"
	"github.com/Code-Hex/synchro/tz"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/walnuts1018/beast/apiserver/domain"
	"github.com/walnuts1018/beast/apiserver/infra/postgres/sqlcgen"
)

// pgtype変換ヘルパー

func toPgTimestamptz(t synchro.Time[tz.UTC]) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.StdTime(), Valid: true}
}

func toPgTimestamptzPtr(t *synchro.Time[tz.UTC]) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.StdTime(), Valid: true}
}

func fromPgTimestamptz(t pgtype.Timestamptz) synchro.Time[tz.UTC] {
	return synchro.In[tz.UTC](t.Time)
}

func fromPgTimestamptzPtr(t pgtype.Timestamptz) *synchro.Time[tz.UTC] {
	if !t.Valid {
		return nil
	}
	v := synchro.In[tz.UTC](t.Time)
	return &v
}

func toPgText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func toPgTextPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func fromPgTextPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func toInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	i := int32(*v)
	return &i
}

func fromInt32Ptr(v *int32) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}

// Video変換

func toDomainVideo(row sqlcgen.Video) domain.Video {
	v := domain.Video{
		ID:               row.ID,
		OwnerUserID:      row.OwnerUserID,
		Status:           domain.VideoStatus(row.Status),
		SourceObjectKey:  row.SourceObjectKey,
		EncodedObjectKey: fromPgTextPtr(row.EncodedObjectKey),
		UploadedAt:       fromPgTimestamptz(row.UploadedAt),
		ReadyAt:          fromPgTimestamptzPtr(row.ReadyAt),
		FailedReason:     fromPgTextPtr(row.FailedReason),
		DurationMillis:   fromInt32Ptr(row.DurationMillis),
		Width:            fromInt32Ptr(row.Width),
		Height:           fromInt32Ptr(row.Height),
		Rating:           fromInt32PtrToRating(row.Rating),
		PlayCount:        int(row.PlayCount),
		LastPlayedAt:     fromPgTimestamptzPtr(row.LastPlayedAt),
		CreatedAt:        fromPgTimestamptz(row.CreatedAt),
		UpdatedAt:        fromPgTimestamptz(row.UpdatedAt),
	}

	if row.PlaybackManifestUrl.Valid && row.PlaybackExpiresAt.Valid && row.PlaybackEncAlgorithm.Valid && row.PlaybackEncKeyVersion != nil {
		v.Playback = &domain.PlaybackGrant{
			VideoID:     row.ID,
			ManifestURL: row.PlaybackManifestUrl.String,
			ExpiresAt:   synchro.In[tz.UTC](row.PlaybackExpiresAt.Time),
			Encryption: domain.EncryptionMetadata{
				Algorithm:        domain.EncryptionAlgorithm(row.PlaybackEncAlgorithm.String),
				KeyVersion:       int(*row.PlaybackEncKeyVersion),
				Nonce:            row.PlaybackEncNonce,
				EncryptedDataKey: row.PlaybackEncEncryptedDataKey,
			},
		}
	}

	if row.ContentEncAlgorithm.Valid && row.ContentEncKeyVersion != nil {
		v.ContentEncryption = &domain.EncryptionMetadata{
			Algorithm:        domain.EncryptionAlgorithm(row.ContentEncAlgorithm.String),
			KeyVersion:       int(*row.ContentEncKeyVersion),
			Nonce:            row.ContentEncNonce,
			EncryptedDataKey: row.ContentEncEncryptedDataKey,
		}
	}

	return v
}

// toCreateVideoParamsとtoUpdateVideoParamsはsqlcが生成する別々の型に対する変換のため、
// 構造が同一でも共通化できない。
//
//nolint:dupl
func toCreateVideoParams(v domain.Video) sqlcgen.CreateVideoParams {
	p := sqlcgen.CreateVideoParams{
		ID:               v.ID,
		OwnerUserID:      v.OwnerUserID,
		Status:           string(v.Status),
		SourceObjectKey:  v.SourceObjectKey,
		EncodedObjectKey: toPgTextPtr(v.EncodedObjectKey),
		UploadedAt:       toPgTimestamptz(v.UploadedAt),
		ReadyAt:          toPgTimestamptzPtr(v.ReadyAt),
		FailedReason:     toPgTextPtr(v.FailedReason),
		DurationMillis:   toInt32Ptr(v.DurationMillis),
		Width:            toInt32Ptr(v.Width),
		Height:           toInt32Ptr(v.Height),
		Rating:           toRatingInt32Ptr(v.Rating),
		PlayCount:        int32(v.PlayCount),
		LastPlayedAt:     toPgTimestamptzPtr(v.LastPlayedAt),
		CreatedAt:        toPgTimestamptz(v.CreatedAt),
		UpdatedAt:        toPgTimestamptz(v.UpdatedAt),
	}

	if v.Playback != nil {
		p.PlaybackManifestUrl = toPgText(v.Playback.ManifestURL)
		p.PlaybackExpiresAt = toPgTimestamptz(v.Playback.ExpiresAt)
		p.PlaybackEncAlgorithm = toPgText(string(v.Playback.Encryption.Algorithm))
		kv := int32(v.Playback.Encryption.KeyVersion)
		p.PlaybackEncKeyVersion = &kv
		p.PlaybackEncNonce = v.Playback.Encryption.Nonce
		p.PlaybackEncEncryptedDataKey = v.Playback.Encryption.EncryptedDataKey
	}

	if v.ContentEncryption != nil {
		p.ContentEncAlgorithm = toPgText(string(v.ContentEncryption.Algorithm))
		kv := int32(v.ContentEncryption.KeyVersion)
		p.ContentEncKeyVersion = &kv
		p.ContentEncNonce = v.ContentEncryption.Nonce
		p.ContentEncEncryptedDataKey = v.ContentEncryption.EncryptedDataKey
	}

	return p
}

//nolint:dupl
func toUpdateVideoParams(v domain.Video) sqlcgen.UpdateVideoParams {
	p := sqlcgen.UpdateVideoParams{
		ID:               v.ID,
		OwnerUserID:      v.OwnerUserID,
		Status:           string(v.Status),
		SourceObjectKey:  v.SourceObjectKey,
		EncodedObjectKey: toPgTextPtr(v.EncodedObjectKey),
		UploadedAt:       toPgTimestamptz(v.UploadedAt),
		ReadyAt:          toPgTimestamptzPtr(v.ReadyAt),
		FailedReason:     toPgTextPtr(v.FailedReason),
		DurationMillis:   toInt32Ptr(v.DurationMillis),
		Width:            toInt32Ptr(v.Width),
		Height:           toInt32Ptr(v.Height),
		CreatedAt:        toPgTimestamptz(v.CreatedAt),
		UpdatedAt:        toPgTimestamptz(v.UpdatedAt),
	}

	if v.Playback != nil {
		p.PlaybackManifestUrl = toPgText(v.Playback.ManifestURL)
		p.PlaybackExpiresAt = toPgTimestamptz(v.Playback.ExpiresAt)
		p.PlaybackEncAlgorithm = toPgText(string(v.Playback.Encryption.Algorithm))
		kv := int32(v.Playback.Encryption.KeyVersion)
		p.PlaybackEncKeyVersion = &kv
		p.PlaybackEncNonce = v.Playback.Encryption.Nonce
		p.PlaybackEncEncryptedDataKey = v.Playback.Encryption.EncryptedDataKey
	}

	if v.ContentEncryption != nil {
		p.ContentEncAlgorithm = toPgText(string(v.ContentEncryption.Algorithm))
		kv := int32(v.ContentEncryption.KeyVersion)
		p.ContentEncKeyVersion = &kv
		p.ContentEncNonce = v.ContentEncryption.Nonce
		p.ContentEncEncryptedDataKey = v.ContentEncryption.EncryptedDataKey
	}

	return p
}

// SharedKeyVersion変換

func toDomainSharedKeyVersion(row sqlcgen.GetSharedKeyVersionRow) domain.SharedKeyVersion {
	return domain.SharedKeyVersion{
		Version:      int(row.Version),
		PublicKeyPEM: row.PublicKeyPem,
		Status:       domain.SharedKeyVersionStatus(row.Status),
		CreatedAt:    fromPgTimestamptz(row.CreatedAt),
		RevokedAt:    fromPgTimestamptzPtr(row.RevokedAt),
	}
}

func toDomainSharedKeyVersionFromList(row sqlcgen.ListSharedKeyVersionsRow) domain.SharedKeyVersion {
	return domain.SharedKeyVersion{
		Version:      int(row.Version),
		PublicKeyPEM: row.PublicKeyPem,
		Status:       domain.SharedKeyVersionStatus(row.Status),
		CreatedAt:    fromPgTimestamptz(row.CreatedAt),
		RevokedAt:    fromPgTimestamptzPtr(row.RevokedAt),
	}
}

func toDomainSharedKeyVersionFromRevoke(row sqlcgen.RevokeSharedKeyVersionRow) domain.SharedKeyVersion {
	return domain.SharedKeyVersion{
		Version:      int(row.Version),
		PublicKeyPEM: row.PublicKeyPem,
		Status:       domain.SharedKeyVersionStatus(row.Status),
		CreatedAt:    fromPgTimestamptz(row.CreatedAt),
		RevokedAt:    fromPgTimestamptzPtr(row.RevokedAt),
	}
}

// DeviceWrappedSharedKey変換

func toDomainDeviceWrappedSharedKey(row sqlcgen.ListDeviceWrappedSharedKeysRow) domain.DeviceWrappedSharedKey {
	return domain.DeviceWrappedSharedKey{
		DeviceID:                  row.DeviceID,
		DevicePublicKeyPEM:        row.DevicePublicKeyPem,
		SharedKeyVersion:          int(row.SharedKeyVersion),
		EncryptedSharedPrivateKey: row.EncryptedSharedPrivateKey,
		CreatedAt:                 fromPgTimestamptz(row.CreatedAt),
	}
}

// UploadSession変換

func toDomainUploadSession(row sqlcgen.UploadSession) domain.UploadSession {
	return domain.UploadSession{
		ID:        row.ID,
		OwnerUser: row.OwnerUser,
		ObjectKey: row.ObjectKey,
		UploadURL: row.UploadUrl,
		ExpiresAt: fromPgTimestamptz(row.ExpiresAt),
		CreatedAt: fromPgTimestamptz(row.CreatedAt),
	}
}

// EncodingProgress変換

func toDomainEncodingProgress(row sqlcgen.EncodingProgress) domain.VideoEncodingProgress {
	return domain.VideoEncodingProgress{
		VideoID:   row.VideoID,
		OwnerUser: row.OwnerUser,
		Status:    domain.VideoStatus(row.Status),
		Percent:   row.Percent,
		UpdatedAt: fromPgTimestamptz(row.UpdatedAt),
		Message:   fromPgTextPtr(row.Message),
	}
}

// Rating変換

func fromInt32PtrToRating(v *int32) *domain.Rating {
	if v == nil {
		return nil
	}
	r := domain.Rating(*v)
	return &r
}

func toRatingInt32Ptr(r *domain.Rating) *int32 {
	if r == nil {
		return nil
	}
	v := int32(*r)
	return &v
}

// PlaybackHistory変換

func toDomainPlaybackHistory(row sqlcgen.PlaybackHistory) domain.PlaybackHistory {
	return domain.PlaybackHistory{
		ID:          row.ID,
		VideoID:     row.VideoID,
		OwnerUserID: row.OwnerUserID,
		PlayedAt:    fromPgTimestamptz(row.PlayedAt),
	}
}
