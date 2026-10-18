package record

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextStripsESC(t *testing.T) {
	got := SanitizeText("hi\x1b[31mx")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ESC retained: %q", got)
	}
}

func TestTextReplacesInvalidUTF8(t *testing.T) {
	got := SanitizeText("a\xffb")
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestTextNewlinesBecomeSpaces(t *testing.T) {
	got := SanitizeText("a\nb")
	if got != "a b" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockPreservesNewlines(t *testing.T) {
	got := SanitizeBlock("a\nb\nc")
	if got != "a\nb\nc" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockNormalizesCRLF(t *testing.T) {
	got := SanitizeBlock("a\r\nb\rc")
	if got != "a\nb\nc" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockStillStripsESC(t *testing.T) {
	got := SanitizeBlock("hi\x1b[31mx\nline2")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ESC retained: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("newline lost: %q", got)
	}
}

func FuzzText(f *testing.F) {
	f.Add("hello")
	f.Add("\x1b[31m")
	f.Add("\r\n\t")
	f.Fuzz(func(t *testing.T, s string) {
		got := SanitizeText(s)
		if strings.ContainsRune(got, '\x1b') {
			t.Fatalf("ESC remained in %q", got)
		}
		for _, r := range got {
			if r < 0x20 && r != '\t' {
				t.Fatalf("control %U remained in %q", r, got)
			}
		}
	})
}

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
