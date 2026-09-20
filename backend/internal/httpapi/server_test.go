package httpapi

import (
	"context"
	"testing"

	"github.com/walnuts1018/beast/backend/internal/domain"
	"github.com/walnuts1018/beast/backend/internal/encoding"
	"github.com/walnuts1018/beast/backend/internal/store"
)

func TestHandleEncodingEventValidatesOwnerAndFinalizesVideo(t *testing.T) {
	repository := store.NewMemory()
	if _, err := repository.RegisterSharedKey(context.Background(), store.SharedKey{ID: "shared-key", OwnerID: "owner", Version: "1", PublicKey: "public", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	video, err := domain.NewVideo("owner", "encrypted-tags", "staging/source", domain.EncryptionMetadata{ChunkSize: 1 << 20, KeyVersion: "1", SharedKeyID: "shared-key"})
	if err != nil {
		t.Fatal(err)
	}
	video.ObjectKey = "videos/" + video.ID + "/dash/manifest.mpd"
	if _, err := repository.CreateVideo(context.Background(), video); err != nil {
		t.Fatal(err)
	}
	video, _ = repository.GetVideo(context.Background(), "owner", video.ID)
	if err := video.TransitionStatus(domain.VideoStatusEncoding); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdateVideo(context.Background(), video); err != nil {
		t.Fatal(err)
	}
	server := &Server{Videos: repository}
	manifest := encoding.Artifact{ObjectKey: video.ObjectKey, Encryption: encoding.EncryptionMetadata{Algorithm: "AES-256-GCM-CHUNKED-RSA-OAEP-SHA256", ChunkSize: 1 << 20, KeyVersion: "1", Nonce: "nonce", EncryptedDataKey: "data-key", SharedKeyID: "shared-key"}}
	event := encoding.Event{ContractVersion: encoding.ContractVersion, VideoID: video.ID, OwnerID: "owner", SourceObjectKey: "staging/source", Status: "READY", Progress: 1, Manifest: manifest, Artifacts: map[string]encoding.Artifact{"manifest.mpd": manifest}}
	if err := server.HandleEncodingEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	ready, err := repository.GetVideo(context.Background(), "owner", video.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != domain.VideoStatusReady || ready.Progress != 1 {
		t.Fatalf("video was not finalized: %#v", ready)
	}
	event.OwnerID = "other-owner"
	if err := server.HandleEncodingEvent(context.Background(), event); err == nil {
		t.Fatal("owner mismatch was accepted")
	}
}
