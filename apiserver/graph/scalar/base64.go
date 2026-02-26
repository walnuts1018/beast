package scalar

import (
	"encoding/base64"
	"fmt"
	"io"
)

type Base64 []byte

func (b Base64) MarshalGQL(w io.Writer) {
	encoded := base64.StdEncoding.EncodeToString(b)
	_, _ = io.WriteString(w, fmt.Sprintf("\"%s\"", encoded))
}

func (b *Base64) UnmarshalGQL(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("base64 scalar must be string")
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}

	*b = Base64(decoded)
	return nil
}
