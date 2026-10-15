package project_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
)

func TestCellsASCII(t *testing.T) {
	if project.Cells("abc") != 3 {
		t.Fatal(project.Cells("abc"))
	}
}

func TestCellsCombining(t *testing.T) {
	s := "e\u0301"
	if project.Cells(s) != 1 {
		t.Fatalf("got %d", project.Cells(s))
	}
}

func TestCellsCJK(t *testing.T) {
	// 你 = CJK, width 2
	if project.Cells("你好") != 4 {
		t.Fatalf("got %d", project.Cells("你好"))
	}
}

func TestCellsEmojiConservative(t *testing.T) {
	// 😀 U+1F600
	if project.Cells("😀") != 2 {
		t.Fatalf("got %d", project.Cells("😀"))
	}
}

func TestCellsEmojiPresentationSequences(t *testing.T) {
	for _, s := range []string{"1️⃣", "©️"} {
		if got := project.Cells(s); got != 2 {
			t.Fatalf("Cells(%q)=%d, want 2", s, got)
		}
	}
}

func TestCellsZWJSequence(t *testing.T) {
	// Family emoji often ZWJ-joined; ZWJ itself is zero-width.
	// Base + ZWJ + base: 2+0+2 = 4
	s := "👨\u200d👩"
	got := project.Cells(s)
	if got < 2 {
		t.Fatalf("got %d", got)
	}
}

func TestTruncateDoesNotSplitCombining(t *testing.T) {
	s := "ae\u0301bc"
	got := project.Truncate(s, 3)
	// should not leave a dangling combining mark without base
	if project.Cells(got) > 3 {
		t.Fatal(got)
	}
}

func TestTruncate(t *testing.T) {
	got := project.Truncate("hello world", 6)
	if got != "hello…" {
		t.Fatal(got)
	}
}

func TestTruncateVisiblePreservesPresentationSequences(t *testing.T) {
	styled := "\x1b[2mhello world\x1b[0m"
	got := project.TruncateVisible(styled, 6)
	if project.VisibleCells(got) != 6 {
		t.Fatalf("visible cells=%d, text=%q", project.VisibleCells(got), got)
	}
	if project.StripANSI(got) != "hello…" {
		t.Fatalf("visible text=%q", project.StripANSI(got))
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("style must be reset after truncation: %q", got)
	}
}

func TestTruncateVisibleClosesHyperlink(t *testing.T) {
	link := "\x1b]8;;https://example.com\x07hello world\x1b]8;;\x07"
	got := project.TruncateVisible(link, 6)
	if project.VisibleCells(got) != 6 {
		t.Fatalf("visible cells=%d, text=%q", project.VisibleCells(got), got)
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
		n := project.Cells(s)
		if n < 0 {
			t.Fatal(n)
		}
		_ = project.Truncate(s, 10)
	})
}
