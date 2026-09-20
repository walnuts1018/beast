package worker

import (
	"testing"
)

func TestDashArgsPrefersStreamCopy(t *testing.T) {
	args := dashArgs("input.mp4", "manifest.mpd", 4, false)
	if want := "copy"; !containsPair(args, "-c", want) {
		t.Fatalf("dash args do not preserve streams: %v", args)
	}
}

func TestDashArgsUsesFallbackEncoding(t *testing.T) {
	args := dashArgs("input.mov", "manifest.mpd", 6, true)
	if !containsPair(args, "-c:v", "libx264") || !containsPair(args, "-c:a", "aac") {
		t.Fatalf("dash args do not contain fallback encoders: %v", args)
	}
}

func TestJobValidate(t *testing.T) {
	tests := []struct {
		name string
		job  Job
		want bool
	}{
		{name: "valid", job: Job{VideoID: "video-1", InputPath: "/tmp/input.mp4"}, want: true},
		{name: "missing id", job: Job{InputPath: "/tmp/input.mp4"}},
		{name: "missing input", job: Job{VideoID: "video-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.job.validate() == nil; got != tt.want {
				t.Fatalf("valid=%v, want %v", got, tt.want)
			}
		})
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
