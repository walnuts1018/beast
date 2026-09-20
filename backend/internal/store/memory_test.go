package store

import (
	"context"
	"testing"

	"github.com/walnuts1018/beast/backend/internal/domain"
)

func TestMemoryUpdateVideoTagsRequiresOwnerAndPreservesVideoMetadata(t *testing.T) {
	ctx := context.Background()
	repository := NewMemory()
	video, err := domain.NewServerVideo("owner", []string{"before"}, "source/object")
	if err != nil {
		t.Fatal(err)
	}
	video.Status = domain.VideoStatusReady
	video.ObjectKey = "hls/manifest.m3u8"
	video.PlayCount = 3
	if _, err := repository.CreateVideo(ctx, video); err != nil {
		t.Fatal(err)
	}

	updated, err := repository.UpdateVideoTags(ctx, "owner", video.ID, []string{"after"})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "after" {
		t.Fatalf("tags = %#v", updated.Tags)
	}
	if updated.Status != domain.VideoStatusReady || updated.ObjectKey != "hls/manifest.m3u8" || updated.PlayCount != 3 {
		t.Fatalf("video metadata changed: %#v", updated)
	}
	if _, err := repository.UpdateVideoTags(ctx, "other-owner", video.ID, []string{"third"}); err == nil {
		t.Fatal("update accepted for another owner")
	}
	if _, err := repository.UpdateVideoTags(ctx, "owner", video.ID, []string{""}); err == nil {
		t.Fatal("empty tag was accepted")
	}
}
