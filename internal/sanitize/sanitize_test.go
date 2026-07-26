package sanitize_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

func TestTextStripsESC(t *testing.T) {
	got := sanitize.Text("hi\x1b[31mx")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ESC retained: %q", got)
	}
}

func TestTextReplacesInvalidUTF8(t *testing.T) {
	got := sanitize.Text("a\xffb")
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestTextNewlinesBecomeSpaces(t *testing.T) {
	got := sanitize.Text("a\nb")
	if got != "a b" {
		t.Fatalf("got %q", got)
	}
}

func FuzzText(f *testing.F) {
	f.Add("hello")
	f.Add("\x1b[31m")
	f.Add("\r\n\t")
	f.Fuzz(func(t *testing.T, s string) {
		_ = sanitize.Text(s)
	})
}
