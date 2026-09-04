package text

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8_DoesNotSplitRune(t *testing.T) {
	// "あ" is 3 bytes. max=5 should keep one rune + suffix, not a partial rune.
	s := "あああ"
	got := TruncateUTF8(s, 5, "…")
	if !utf8.ValidString(got) {
		t.Fatalf("invalid utf8: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("want suffix: %q", got)
	}
	// First rune (3 bytes) fits in budget 5; second does not.
	if !strings.HasPrefix(got, "あ") {
		t.Fatalf("want leading あ: %q", got)
	}
	// Byte count of body before suffix is a complete rune boundary.
	body := strings.TrimSuffix(got, "…")
	if !utf8.ValidString(body) || body != "あ" {
		t.Fatalf("body=%q want あ", body)
	}
}

func TestTruncateUTF8_ShortUnchanged(t *testing.T) {
	if got := TruncateUTF8("hi", 10, "…"); got != "hi" {
		t.Fatalf("got %q", got)
	}
}
