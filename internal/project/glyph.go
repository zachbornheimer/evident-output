package project

import (
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// carriesProblem reports a row that succeeded with a Problem recorded: its
// "✓" would read as plain success beside its own Problem.
func carriesProblem(state record.State, problems []record.Problem) bool {
	return state.Succeeded() && len(problems) > 0
}

// RowGlyph is the glyph of a Task's row: its state's, except that a success
// carrying a Problem draws the attention bang.
func RowGlyph(state record.State, problems []record.Problem, profile GlyphProfile) string {
	if carriesProblem(state, problems) {
		return GlyphWarningState.Render(profile)
	}
	return TaskGlyph(state, profile)
}

// RowColor is the SGR code of RowGlyph's glyph.
func RowColor(state record.State, problems []record.Problem) string {
	if carriesProblem(state, problems) {
		return SGRYellow
	}
	return StateColor(state)
}

// TaskGlyph maps a Task's state to its glyph in the given profile. Cancelled
// gets its own face (GlyphCancelled) per the tightened vocabulary table —
// it must be visually distinct from a task that never got attention.
// Skipped keeps Pending's glyph, unchanged from before the table existed.
func TaskGlyph(s record.State, profile GlyphProfile) string {
	switch s {
	case record.Succeeded, record.Satisfied:
		return GlyphDone.Render(profile)
	case record.Failed:
		return GlyphFailedState.Render(profile)
	case record.Refused:
		return GlyphBlockedState.Render(profile)
	case record.Running:
		return GlyphRunning.Render(profile)
	case record.Pending, record.Excluded:
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
