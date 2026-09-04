package evo

import (
	"strings"
	"testing"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestFormatLivePaneLine_OverflowTruncatesInsteadOfDroppingFields reproduces
// gate-7 finding 2: when the full history-grammar line (time + level + msg +
// attrs) overflows the pane's column budget, formatLivePaneLine rebuilt the
// record with Fields set to nil — silently claiming the record had no
// structured attributes at all, with no marker that anything was cut. The
// fix truncates the full line to the column budget with a trailing "…" so
// the pane never lies about a record's shape.
func TestFormatLivePaneLine_OverflowTruncatesInsteadOfDroppingFields(t *testing.T) {
	rec := debugRecord{
		Level:   "DEBUG",
		Message: "package index loaded",
		Fields: []Field{
			{Key: "packages", Value: 18},
			{Key: "path", Value: "/very/long/repository/path/that/pushes/this/record/past/the/column/budget"},
		},
	}
	full := formatHistoryLine(rec, false)
	columns := txt.VisibleCells(full) - 10 // force overflow, but leave room for some attrs

	got := formatLivePaneLine(rec, columns)

	if txt.VisibleCells(got) > columns {
		t.Fatalf("expected the pane line to respect the column budget (%d), got %d cells: %q", columns, txt.VisibleCells(got), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected a truncation marker on an overflowing pane line, got %q", got)
	}
	if !strings.Contains(got, "package index loaded") {
		t.Fatalf("expected the message to survive truncation, got %q", got)
	}
}
