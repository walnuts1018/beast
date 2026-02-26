package graph

import (
	"github.com/walnuts1018/beast/apiserver/domain"
	"github.com/walnuts1018/beast/apiserver/graph/model"
	"github.com/walnuts1018/beast/apiserver/graph/scalar"
)

func toModelVideoStatus(status domain.VideoStatus) model.VideoStatus {
	return model.VideoStatus(status)
}

func toModelEncryptionAlgorithm(alg domain.EncryptionAlgorithm) model.EncryptionAlgorithm {
	return model.EncryptionAlgorithm(alg)
}

func toDomainVideoStatus(status model.VideoStatus) domain.VideoStatus {
	return domain.VideoStatus(status)
}

func toDomainEncryptionMetadata(input model.EncryptionMetadataInput) domain.EncryptionMetadata {
	return domain.EncryptionMetadata{
		Algorithm:        domain.EncryptionAlgorithm(input.Algorithm),
		KeyVersion:       input.KeyVersion,
		Nonce:            []byte(input.Nonce),
		EncryptedDataKey: []byte(input.EncryptedDataKey),
	}
}

func toModelEncryptionMetadata(item domain.EncryptionMetadata) *model.EncryptionMetadata {
	return &model.EncryptionMetadata{
		Algorithm:        toModelEncryptionAlgorithm(item.Algorithm),
		KeyVersion:       item.KeyVersion,
		Nonce:            scalar.Base64(item.Nonce),
		EncryptedDataKey: scalar.Base64(item.EncryptedDataKey),
	}
}

func toModelSharedKeyVersion(item domain.SharedKeyVersion) *model.SharedKeyVersion {
	return &model.SharedKeyVersion{
		Version:      item.Version,
		PublicKeyPem: item.PublicKeyPEM,
		Status:       model.SharedKeyVersionStatus(item.Status),
		CreatedAt:    item.CreatedAt,
		RevokedAt:    item.RevokedAt,
	}
}

func toModelDeviceWrappedSharedKey(item domain.DeviceWrappedSharedKey) *model.DeviceWrappedSharedKey {
	return &model.DeviceWrappedSharedKey{
		DeviceID:                  item.DeviceID,
		SharedKeyVersion:          item.SharedKeyVersion,
		EncryptedSharedPrivateKey: scalar.Base64(item.EncryptedSharedPrivateKey),
		CreatedAt:                 item.CreatedAt,
	}
}

func toModelUploadSession(item domain.UploadSession) *model.UploadSession {
	return &model.UploadSession{
		ID:        item.ID,
		ObjectKey: item.ObjectKey,
		UploadURL: item.UploadURL,
		ExpiresAt: item.ExpiresAt,
	}
}

func toModelProgress(item domain.VideoEncodingProgress) *model.VideoEncodingProgress {
	return &model.VideoEncodingProgress{
		VideoID:   item.VideoID,
		Status:    toModelVideoStatus(item.Status),
		Percent:   item.Percent,
		UpdatedAt: item.UpdatedAt,
		Message:   item.Message,
	}
}

func toModelPlaybackGrant(item *domain.PlaybackGrant) *model.PlaybackGrant {
	if item == nil {
		return nil
	}

	return &model.PlaybackGrant{
		VideoID:     item.VideoID,
		ManifestURL: item.ManifestURL,
		ExpiresAt:   item.ExpiresAt,
		Encryption:  toModelEncryptionMetadata(item.Encryption),
	}
}

func toModelVideo(item domain.Video) *model.Video {
	return &model.Video{
		ID:                item.ID,
		OwnerUserID:       item.OwnerUserID,
		Status:            toModelVideoStatus(item.Status),
		UploadedAt:        item.UploadedAt,
		ReadyAt:           item.ReadyAt,
		FailedReason:      item.FailedReason,
		DurationMillis:    item.DurationMillis,
		Width:             item.Width,
		Height:            item.Height,
		Playback:          toModelPlaybackGrant(item.Playback),
		EncryptedTags:     scalar.Base64(item.EncryptedTags),
		TagEncryption:     toModelEncryptionMetadata(item.TagEncryption),
		ContentEncryption: toModelEncryptionMetadataPtr(item.ContentEncryption),
	}
}

func toModelEncryptionMetadataPtr(item *domain.EncryptionMetadata) *model.EncryptionMetadata {
	if item == nil {
		return nil
	}

	return toModelEncryptionMetadata(*item)
}

func toModelMe(item domain.Me) *model.Me {
	shared := make([]*model.SharedKeyVersion, 0, len(item.SharedKeyVersions))
	for _, v := range item.SharedKeyVersions {
		shared = append(shared, toModelSharedKeyVersion(v))
	}

	wrapped := make([]*model.DeviceWrappedSharedKey, 0, len(item.DeviceWrappedSharedKeys))
	for _, w := range item.DeviceWrappedSharedKeys {
		wrapped = append(wrapped, toModelDeviceWrappedSharedKey(w))
	}

	return &model.Me{
		UserID:                  item.UserID,
		SharedKeyVersions:       shared,
		DeviceWrappedSharedKeys: wrapped,
	}
}

func toModelVideoConnection(conn domain.VideoConnection) *model.VideoConnection {
	edges := make([]*model.VideoEdge, 0, len(conn.Edges))
	for _, e := range conn.Edges {
		edges = append(edges, &model.VideoEdge{Cursor: e.Cursor, Node: toModelVideo(e.Node)})
	}

	return &model.VideoConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage: conn.HasNext,
			EndCursor:   conn.NextCursor,
		},
	}
}
