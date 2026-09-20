package worker

import (
	"strings"
	"testing"
)

func TestHLSArgsUsesFragmentedMP4AndForcedKeyframesOnFallback(t *testing.T) {
	args := hlsArgs("input.mov", "/tmp/manifest.m3u8", 4, true)
	if !containsPair(args, "-hls_segment_type", "fmp4") || !containsPair(args, "-hls_time", "4") || !containsPair(args, "-force_key_frames", "expr:gte(t,n_forced*4)") {
		t.Fatalf("HLS args are incomplete: %v", args)
	}
}

func TestCodecsAllowStreamCopy(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
		want bool
	}{
		{name: "h264 aac", json: `{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}]}`, want: true},
		{name: "hevc aac", json: `{"streams":[{"codec_type":"video","codec_name":"hevc"},{"codec_type":"audio","codec_name":"aac"}]}`, want: true},
		{name: "av1 no audio", json: `{"streams":[{"codec_type":"video","codec_name":"av1"}]}`, want: true},
		{name: "vp9 aac", json: `{"streams":[{"codec_type":"video","codec_name":"vp9"},{"codec_type":"audio","codec_name":"aac"}]}`, want: false},
		{name: "h264 opus", json: `{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"opus"}]}`, want: false},
		{name: "unsupported second audio", json: `{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"},{"codec_type":"audio","codec_name":"opus"}]}`, want: false},
		{name: "unsupported second video", json: `{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"video","codec_name":"vp9"}]}`, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := codecsAllowStreamCopy([]byte(test.json))
			if err != nil || got != test.want {
				t.Fatalf("copy compatibility = %v, err=%v, want=%v", got, err, test.want)
			}
		})
	}
}

func TestHLSArgsHasIndependentSegments(t *testing.T) {
	args := strings.Join(hlsArgs("input", "manifest.m3u8", 3, false), " ")
	if !strings.Contains(args, "-hls_flags independent_segments") {
		t.Fatalf("HLS args omit independent segments: %s", args)
	}
}

func TestJobValidate(t *testing.T) {
	valid := Job{ContractVersion: ContractVersion, VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "staging/source", OutputPrefix: "videos/video-1/hls"}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	valid.VideoID = ""
	if err := valid.validate(); err == nil {
		t.Fatal("missing video ID was accepted")
	}
}

func containsPair(args []string, key, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return true
		}
	}
	return false
}
