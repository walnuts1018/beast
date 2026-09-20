package store

import (
	"testing"

	"github.com/walnuts1018/beast/backend/internal/crypto"
)

func TestNormalizedChunkSize(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int32
	}{
		{name: "staging default", input: 0, want: crypto.ChunkSize},
		{name: "negative default", input: -1, want: crypto.ChunkSize},
		{name: "configured", input: 64 * 1024, want: 64 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizedChunkSize(tt.input); got != tt.want {
				t.Fatalf("normalizedChunkSize(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
