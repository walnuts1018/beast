package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	MaxTags      = 8
	MaxTagLength = 64
)

func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) > MaxTags {
		return nil, errors.New("at most eight tags are allowed")
	}
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, value := range tags {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("tags must not be empty")
		}
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > MaxTagLength {
			return nil, errors.New("tag length is invalid")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
