package width_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/width"
)

func TestCellsASCII(t *testing.T) {
	if width.Cells("abc") != 3 {
		t.Fatal(width.Cells("abc"))
	}
}

func TestCellsCombining(t *testing.T) {
	// e + combining acute
	s := "e\u0301"
	if width.Cells(s) != 1 {
		t.Fatalf("got %d", width.Cells(s))
	}
}

func TestTruncate(t *testing.T) {
	got := width.Truncate("hello world", 6)
	if got != "hello…" {
		t.Fatal(got)
	}
}
