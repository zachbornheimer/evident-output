package width_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/width"
)

func TestCellsASCII(t *testing.T) {
	if width.Cells("abc") != 3 {
		t.Fatal(width.Cells("abc"))
	}
}

func TestCellsCombining(t *testing.T) {
	s := "e\u0301"
	if width.Cells(s) != 1 {
		t.Fatalf("got %d", width.Cells(s))
	}
}

func TestCellsCJK(t *testing.T) {
	// 你 = CJK, width 2
	if width.Cells("你好") != 4 {
		t.Fatalf("got %d", width.Cells("你好"))
	}
}

func TestCellsEmojiConservative(t *testing.T) {
	// 😀 U+1F600
	if width.Cells("😀") != 2 {
		t.Fatalf("got %d", width.Cells("😀"))
	}
}

func TestCellsEmojiPresentationSequences(t *testing.T) {
	for _, text := range []string{"1️⃣", "©️"} {
		if got := width.Cells(text); got != 2 {
			t.Fatalf("Cells(%q)=%d, want 2", text, got)
		}
	}
}

func TestCellsZWJSequence(t *testing.T) {
	// Family emoji often ZWJ-joined; ZWJ itself is zero-width.
	// Base + ZWJ + base: 2+0+2 = 4
	s := "👨\u200d👩"
	got := width.Cells(s)
	if got < 2 {
		t.Fatalf("got %d", got)
	}
}

func TestTruncateDoesNotSplitCombining(t *testing.T) {
	s := "ae\u0301bc"
	got := width.Truncate(s, 3)
	// should not leave a dangling combining mark without base
	if width.Cells(got) > 3 {
		t.Fatal(got)
	}
}

func TestTruncate(t *testing.T) {
	got := width.Truncate("hello world", 6)
	if got != "hello…" {
		t.Fatal(got)
	}
}

func TestTruncateVisiblePreservesPresentationSequences(t *testing.T) {
	styled := "\x1b[2mhello world\x1b[0m"
	got := width.TruncateVisible(styled, 6)
	if width.VisibleCells(got) != 6 {
		t.Fatalf("visible cells=%d, text=%q", width.VisibleCells(got), got)
	}
	if width.StripANSI(got) != "hello…" {
		t.Fatalf("visible text=%q", width.StripANSI(got))
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("style must be reset after truncation: %q", got)
	}
}

func TestTruncateVisibleClosesHyperlink(t *testing.T) {
	link := "\x1b]8;;https://example.com\x07hello world\x1b]8;;\x07"
	got := width.TruncateVisible(link, 6)
	if width.VisibleCells(got) != 6 {
		t.Fatalf("visible cells=%d, text=%q", width.VisibleCells(got), got)
	}
	if !strings.Contains(got, "\x1b]8;;\x1b\\") {
		t.Fatalf("hyperlink must be closed after truncation: %q", got)
	}
}

func FuzzCells(f *testing.F) {
	f.Add("hello")
	f.Add("你好")
	f.Add("😀")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, s string) {
		n := width.Cells(s)
		if n < 0 {
			t.Fatal(n)
		}
		_ = width.Truncate(s, 10)
	})
}
