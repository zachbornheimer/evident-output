package render

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Style is how one human render paints: whether it may color, whether it
// shows verbose detail, and which glyph profile it draws from. Every row
// writer takes one Style instead of the three values separately.
type Style struct {
	Color   bool
	Verbose bool
	Profile txt.GlyphProfile
}

// Dim demotes subordinate text.
func (s Style) Dim(text string) string { return txt.Dim(text, s.Color) }

// Paint applies an SGR code to text.
func (s Style) Paint(text, sgr string) string { return txt.Style(text, sgr, s.Color) }

// StateGlyph is a Task or container state's glyph in its state color.
func (s Style) StateGlyph(state core.EntityState) string {
	return txt.StyleGlyph(TaskGlyph(state, s.Profile), StateColor(state), s.Color)
}

// WarningGlyph is the yellow attention bang ("!").
func (s Style) WarningGlyph() string {
	return txt.StyleGlyph(txt.GlyphWarningState.Render(s.Profile), txt.SGRYellow, s.Color)
}

// EvidenceGlyph is the dim evidence connector ("└─").
func (s Style) EvidenceGlyph() string { return s.Dim(txt.GlyphEvidence.Render(s.Profile)) }

// OverflowGlyph is the dim omission marker ("…").
func (s Style) OverflowGlyph() string { return s.Dim(txt.GlyphOverflow.Render(s.Profile)) }

// Emphasized keeps evidence at full intensity when emphasize is set, and
// demotes it otherwise — the one place that decides "is this text
// decoration or evidence" for the problem-rendering chain.
func (s Style) Emphasized(text string, emphasize bool) string {
	if emphasize {
		return text
	}
	return s.Dim(text)
}

func StateColor(s core.EntityState) string {
	switch s {
	case core.Done:
		return txt.SGRGreen
	case core.Failed:
		return txt.SGRRed
	case core.Blocked:
		return txt.SGRRed
	case core.Running:
		return txt.SGRCyan
	case core.Pending, core.Skipped, core.Cancelled, core.Incomplete, core.NotStarted:
		return txt.SGRDim
	default:
		return ""
	}
}

func ConclusionColor(s core.ConclusionState) string {
	switch s {
	case core.StateReady, core.StateChanged:
		return txt.SGRGreen
	case core.StatePlanned:
		return txt.SGRBlue
	case core.StateFailed:
		return txt.SGRRed
	case core.StateBlocked:
		return txt.SGRRed
	case core.StateCancelled:
		return txt.SGRDim
	default:
		return txt.SGRCyan
	}
}

func EffectColor(kind string) string {
	switch kind {
	case "changed":
		return txt.SGRGreen
	case "planned":
		return txt.SGRBlue
	default:
		return txt.SGRCyan
	}
}
