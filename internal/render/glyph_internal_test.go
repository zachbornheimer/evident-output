package render

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestTaskGlyph_ASCIITable pins the full task-state → ASCII face mapping —
// including Cancelled ([cancel]), which is only reachable in practice via a
// Confirm gate cancelled mid-prompt and is easier to pin at the table level
// than to reproduce end-to-end.
func TestTaskGlyph_ASCIITable(t *testing.T) {
	cases := map[core.EntityState]string{
		core.Done:       "[ok]",
		core.Failed:     "[x]",
		core.Blocked:    "[blocked]",
		core.Pending:    "[.]",
		core.Cancelled:  "[cancel]", // evo-rec.md: Cancelled gets its own face, no longer Pending's
		core.Skipped:    "[.]",
		core.NotStarted: "[-]",
		core.Incomplete: "[-]", // an unresolved task at Finish reads as "not started"
	}
	for state, want := range cases {
		if got := TaskGlyph(state, txt.GlyphsASCII); got != want {
			t.Errorf("TaskGlyph(%s, ASCII) = %q, want %q", state, got, want)
		}
	}
}

// TestTaskGlyph_UnicodeUnchanged pins TaskGlyph's pre-existing Unicode faces.
func TestTaskGlyph_UnicodeUnchanged(t *testing.T) {
	cases := map[core.EntityState]string{
		core.Done:       "✓",
		core.Failed:     "✗",
		core.Blocked:    "⊘",
		core.Pending:    "○",
		core.Cancelled:  "■", // evo-rec.md: Cancelled gets its own face, no longer Pending's
		core.Skipped:    "○",
		core.NotStarted: "-",
		core.Incomplete: "-", // an unresolved task at Finish reads as "not started"
	}
	for state, want := range cases {
		if got := TaskGlyph(state, txt.GlyphsUnicode); got != want {
			t.Errorf("TaskGlyph(%s, Unicode) = %q, want %q", state, got, want)
		}
	}
}
