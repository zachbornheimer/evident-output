package project

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// TestFormatLivePaneLine_OverflowTruncatesInsteadOfDroppingFields reproduces
// gate-7 finding 2: when the full history-grammar line (time + level + msg +
// attrs) overflows the pane's column budget, formatLivePaneLine rebuilt the
// record with Fields set to nil — silently claiming the record had no
// structured attributes at all, with no marker that anything was cut. The
// fix truncates the full line to the column budget with a trailing "…" so
// the pane never lies about a record's shape.
func TestFormatLivePaneLine_OverflowTruncatesInsteadOfDroppingFields(t *testing.T) {
	rec := record.DebugRecord{
		Level:   "DEBUG",
		Message: "package index loaded",
		Fields: []record.Field{
			{Key: "packages", Value: 18},
			{Key: "path", Value: "/very/long/repository/path/that/pushes/this/record/past/the/column/budget"},
		},
	}
	full := FormatHistoryLine(rec, false)
	columns := VisibleCells(full) - 10 // force overflow, but leave room for some attrs

	got := formatLivePaneLine(rec, columns)

	if VisibleCells(got) > columns {
		t.Fatalf("expected the pane line to respect the column budget (%d), got %d cells: %q", columns, VisibleCells(got), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected a truncation marker on an overflowing pane line, got %q", got)
	}
	if !strings.Contains(got, "package index loaded") {
		t.Fatalf("expected the message to survive truncation, got %q", got)
	}
}

// TestFormatHistoryLine_WarnErrorReuseEntityGlyphPalette is release-gate
// round 9 finding 3: the debug journal's level token colored every level
// the same cyan, so a WARN or ERROR record read no differently than an INFO
// one in a scrollback that mixes all four. WARN and ERROR now reuse
// stateColor's Warning/Failed colors (sgrYellow/sgrRed) — the same severity
// vocabulary a Task's own row already uses. DEBUG/INFO keep the journal's
// existing cyan.
func TestFormatHistoryLine_WarnErrorReuseEntityGlyphPalette(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"DEBUG", SGRCyan},
		{"INFO", SGRCyan},
		{"WARN", SGRYellow},
		{"ERROR", SGRRed},
	}
	for _, tc := range cases {
		rec := record.DebugRecord{Level: tc.level, Message: "m"}
		line := FormatHistoryLine(rec, true)
		if !strings.Contains(line, tc.want) {
			t.Fatalf("level %s: want SGR %q in rendered line, got %q", tc.level, tc.want, line)
		}
	}
}
