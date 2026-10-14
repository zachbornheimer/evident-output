package project

import (
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// TestTaskGlyph_ASCIITable pins the full task-state → ASCII face mapping —
// including Cancelled ([cancel]), which is only reachable in practice via a
// Confirm gate cancelled mid-prompt and is easier to pin at the table level
// than to reproduce end-to-end.
func TestTaskGlyph_ASCIITable(t *testing.T) {
	cases := map[record.State]string{
		record.Succeeded:  "[ok]",
		record.Failed:     "[x]",
		record.Refused:    "[refused]",
		record.Pending:    "[.]",
		record.Cancelled:  "[cancel]", // evo-rec.md: Cancelled gets its own face, no longer Pending's
		record.Excluded:   "[.]",
		record.NotStarted: "[-]",
		record.Incomplete: "[-]", // an unresolved task at Finish reads as "not started"
	}
	for state, want := range cases {
		if got := TaskGlyph(state, GlyphsASCII); got != want {
			t.Errorf("TaskGlyph(%s, ASCII) = %q, want %q", state, got, want)
		}
	}
}

// TestTaskGlyph_UnicodeUnchanged pins TaskGlyph's pre-existing Unicode faces.
func TestTaskGlyph_UnicodeUnchanged(t *testing.T) {
	cases := map[record.State]string{
		record.Succeeded:  "✓",
		record.Failed:     "✗",
		record.Refused:    "⊘",
		record.Pending:    "○",
		record.Cancelled:  "■", // evo-rec.md: Cancelled gets its own face, no longer Pending's
		record.Excluded:   "○",
		record.NotStarted: "-",
		record.Incomplete: "-", // an unresolved task at Finish reads as "not started"
	}
	for state, want := range cases {
		if got := TaskGlyph(state, GlyphsUnicode); got != want {
			t.Errorf("TaskGlyph(%s, Unicode) = %q, want %q", state, got, want)
		}
	}
}
