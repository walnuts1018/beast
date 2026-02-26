package worker

import (
	"reflect"
	"testing"
)

func TestBuildInitialPlan_CopyBoth(t *testing.T) {
	w := &Worker{}
	meta := mediaMeta{VideoCodec: "h264", AudioCodec: "aac"}
	plan := w.buildInitialPlan(meta, nil)
	if !plan.copyVideo {
		t.Fatalf("expected copyVideo=true")
	}
	if !plan.copyAudio {
		t.Fatalf("expected copyAudio=true")
	}
}

func TestBuildInitialPlan_ReencodeWhenUnknownCodec(t *testing.T) {
	w := &Worker{}
	meta := mediaMeta{VideoCodec: "prores", AudioCodec: "pcm_s16le"}
	plan := w.buildInitialPlan(meta, nil)
	if plan.copyVideo {
		t.Fatalf("expected copyVideo=false")
	}
	if plan.copyAudio {
		t.Fatalf("expected copyAudio=false")
	}
}

func TestBuildInitialPlan_NoAudioStream(t *testing.T) {
	w := &Worker{}
	meta := mediaMeta{VideoCodec: "hevc", AudioCodec: ""}
	plan := w.buildInitialPlan(meta, nil)
	if !plan.copyVideo {
		t.Fatalf("expected copyVideo=true")
	}
	if !plan.copyAudio {
		t.Fatalf("expected copyAudio=true when no audio stream")
	}
}

func TestBuildInitialPlan_ReencodePrefersNVENCOrQSV(t *testing.T) {
	w := &Worker{}
	meta := mediaMeta{VideoCodec: "prores", AudioCodec: "pcm_s16le"}
	encoders := map[string]struct{}{
		"h264_qsv": {},
		"libx264":  {},
	}

	plan := w.buildInitialPlan(meta, encoders)
	if plan.copyVideo {
		t.Fatalf("expected copyVideo=false")
	}
	if plan.videoEncoder != "h264_qsv" {
		t.Fatalf("expected qsv preferred, got: %s", plan.videoEncoder)
	}
}

func TestChooseBestVideoEncoder(t *testing.T) {
	encoders := map[string]struct{}{
		"libx264":    {},
		"h264_nvenc": {},
	}
	if got := chooseBestVideoEncoder(encoders); got != "h264_nvenc" {
		t.Fatalf("unexpected video encoder: %s", got)
	}
}

func TestChooseBestAudioEncoder(t *testing.T) {
	encoders := map[string]struct{}{
		"libopus": {},
	}
	if got := chooseBestAudioEncoder(encoders); got != "libopus" {
		t.Fatalf("unexpected audio encoder: %s", got)
	}
}

func TestBuildFFmpegDashArgs_Copy(t *testing.T) {
	w := &Worker{dashSegmentSeconds: 4}
	plan := ffmpegPlan{copyVideo: true, copyAudio: true}
	args := w.buildFFmpegDashArgs("in.mp4", "out.mpd", plan)

	if !containsPair(args, "-c:v", "copy") {
		t.Fatalf("expected -c:v copy")
	}
	if !containsPair(args, "-c:a", "copy") {
		t.Fatalf("expected -c:a copy")
	}
}

func TestBuildFFmpegDashArgs_Reencode(t *testing.T) {
	w := &Worker{dashSegmentSeconds: 5}
	plan := ffmpegPlan{copyVideo: false, copyAudio: false, videoEncoder: "libx264", audioEncoder: "aac"}
	args := w.buildFFmpegDashArgs("in.mp4", "out.mpd", plan)

	if !containsPair(args, "-c:v", "libx264") {
		t.Fatalf("expected -c:v libx264")
	}
	if !containsPair(args, "-c:a", "aac") {
		t.Fatalf("expected -c:a aac")
	}
	if !containsPair(args, "-seg_duration", "5") {
		t.Fatalf("expected custom segment duration")
	}
}

func TestVideoEncoderOptions(t *testing.T) {
	got := videoEncoderOptions("libx264")
	want := []string{"-preset", "veryfast", "-crf", "22"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected options: %+v", got)
	}
}

func containsPair(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
