package project

import (
	"github.com/zachbornheimer/evident-output/internal/record"
)

// TaskGlyph maps a Task's state to its glyph in the given profile. Cancelled
// gets its own face (GlyphCancelled) per the tightened vocabulary table —
// it must be visually distinct from a task that never got attention.
// Skipped keeps Pending's glyph, unchanged from before the table existed.
func TaskGlyph(s record.EntityState, profile GlyphProfile) string {
	switch s {
	case record.Done:
		return GlyphDone.Render(profile)
	case record.Failed:
		return GlyphFailedState.Render(profile)
	case record.Blocked:
		return GlyphBlockedState.Render(profile)
	case record.Running:
		return GlyphRunning.Render(profile)
	case record.Pending, record.Skipped:
		return GlyphPending.Render(profile)
	case record.Cancelled:
		return GlyphCancelled.Render(profile)
	// Incomplete: a task Finish left non-terminal (never ran or never
	// resolved) has no dedicated row in the vocabulary table; it reads as
	// "not started" rather than the unclassified "·" no state should ever
	// need (evo-rec.md "Conclusion algebra").
	case record.NotStarted, record.Incomplete:
		return GlyphNotStarted.Render(profile)
	default:
		return GlyphUnclassified.Render(profile)
	}
}
