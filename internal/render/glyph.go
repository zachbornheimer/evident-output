// Package render is evident-output's presentation machinery: plain,
// structured (JSON/JSONL), and interactive (live) projection of a
// internal/core Snapshot. Imports core and internal/text; never imports the
// root package (see internal/core's package doc for why — root imports
// render and delegates, so render importing root back would cycle).
package render

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TaskGlyph maps a Task's state to its glyph in the given profile. Cancelled
// gets its own face (GlyphCancelled) per the tightened vocabulary table —
// it must be visually distinct from a task that never got attention.
// Skipped keeps Pending's glyph, unchanged from before the table existed.
func TaskGlyph(s core.EntityState, profile txt.GlyphProfile) string {
	switch s {
	case core.Done:
		return txt.GlyphDone.Render(profile)
	case core.Failed:
		return txt.GlyphFailedState.Render(profile)
	case core.Blocked:
		return txt.GlyphBlockedState.Render(profile)
	case core.Warning:
		return txt.GlyphWarningState.Render(profile)
	case core.Running:
		return txt.GlyphRunning.Render(profile)
	case core.Pending, core.Skipped:
		return txt.GlyphPending.Render(profile)
	case core.Cancelled:
		return txt.GlyphCancelled.Render(profile)
	// Incomplete: a task Finish left non-terminal (never ran or never
	// resolved) has no dedicated row in the vocabulary table; it reads as
	// "not started" rather than the unclassified "·" no state should ever
	// need (evo-rec.md "Conclusion algebra").
	case core.NotStarted, core.Incomplete:
		return txt.GlyphNotStarted.Render(profile)
	default:
		return txt.GlyphUnclassified.Render(profile)
	}
}
