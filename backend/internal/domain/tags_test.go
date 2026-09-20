package domain

import "testing"

func TestNormalizeTagsTrimsAndDeduplicates(t *testing.T) {
	got, err := NormalizeTags([]string{" family ", "family", "旅行"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "family" || got[1] != "旅行" {
		t.Fatalf("normalized tags = %#v", got)
	}
}

func TestNormalizeTagsRejectsTooManyTags(t *testing.T) {
	if _, err := NormalizeTags([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}); err == nil {
		t.Fatal("too many tags were accepted")
	}
}
