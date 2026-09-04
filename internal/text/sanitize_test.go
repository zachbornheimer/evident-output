package text_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/text"
)

func TestTextStripsESC(t *testing.T) {
	got := text.Text("hi\x1b[31mx")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ESC retained: %q", got)
	}
}

func TestTextReplacesInvalidUTF8(t *testing.T) {
	got := text.Text("a\xffb")
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestTextNewlinesBecomeSpaces(t *testing.T) {
	got := text.Text("a\nb")
	if got != "a b" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockPreservesNewlines(t *testing.T) {
	got := text.Block("a\nb\nc")
	if got != "a\nb\nc" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockNormalizesCRLF(t *testing.T) {
	got := text.Block("a\r\nb\rc")
	if got != "a\nb\nc" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockStillStripsESC(t *testing.T) {
	got := text.Block("hi\x1b[31mx\nline2")
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
		got := text.Text(s)
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
