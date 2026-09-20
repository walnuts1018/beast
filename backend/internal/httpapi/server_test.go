package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/walnuts1018/beast/backend/internal/crypto"
	"github.com/walnuts1018/beast/backend/internal/domain"
	"github.com/walnuts1018/beast/backend/internal/encoding"
	"github.com/walnuts1018/beast/backend/internal/store"
)

func TestHandleEncodingEventValidatesOwnerAndFinalizesVideo(t *testing.T) {
	repository := store.NewMemory()
	video, err := domain.NewServerVideo("owner", nil, "staging/source")
	if err != nil {
		t.Fatal(err)
	}
	video.ObjectKey = "videos/" + video.ID + "/hls/manifest.m3u8"
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
	manifest := encoding.Artifact{ObjectKey: video.ObjectKey, Encryption: encoding.EncryptionMetadata{Algorithm: crypto.AtRestAlgorithm, ChunkSize: crypto.AtRestChunkSize, Nonce: "AAAAAAAAAAAAAAAA", EncryptedDataKey: "data-key", PlaintextSize: 10}}
	event := encoding.Event{ContractVersion: encoding.ContractVersion, VideoID: video.ID, OwnerID: "owner", SourceObjectKey: "staging/source", Status: "READY", Progress: 1, Manifest: manifest, Artifacts: map[string]encoding.Artifact{"manifest.m3u8": manifest}}
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

func TestRequestedRangeSupportsMediaSeekPatterns(t *testing.T) {
	for _, test := range []struct {
		rangeValue string
		start      int64
		end        int64
	}{
		{rangeValue: "bytes=0-1023", start: 0, end: 1024},
		{rangeValue: "bytes=1048576-", start: 1048576, end: 4000000},
		{rangeValue: "bytes=-512", start: 3999488, end: 4000000},
	} {
		t.Run(test.rangeValue, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set("Range", test.rangeValue)
			start, end, partial, err := requestedRange(request, 4000000)
			if err != nil || !partial || start != test.start || end != test.end {
				t.Fatalf("range = %d-%d partial=%v err=%v", start, end, partial, err)
			}
		})
	}
}
