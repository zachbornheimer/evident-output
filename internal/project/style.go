package project

import (
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Style is how one human render paints: whether it may color, whether it
// shows verbose detail, and which glyph profile it draws from. Every row
// writer takes one Style instead of the three values separately.
type Style struct {
	Color   bool
	Verbose bool
	Profile GlyphProfile
}

// Dim demotes subordinate text.
func (s Style) Dim(text string) string { return Dim(text, s.Color) }

// Paint applies an SGR code to text.
func (s Style) Paint(text, sgr string) string { return ApplySGR(text, sgr, s.Color) }

// StateGlyph is a Task or container state's glyph in its state color.
func (s Style) StateGlyph(state record.State) string {
	return StyleGlyph(TaskGlyph(state, s.Profile), StateColor(state), s.Color)
}

// WarningGlyph is the yellow attention bang ("!").
func (s Style) WarningGlyph() string {
	return StyleGlyph(GlyphWarningState.Render(s.Profile), SGRYellow, s.Color)
}

// EvidenceGlyph is the dim evidence connector ("└─").
func (s Style) EvidenceGlyph() string { return s.Dim(GlyphEvidence.Render(s.Profile)) }

// OverflowGlyph is the dim omission marker ("…").
func (s Style) OverflowGlyph() string { return s.Dim(GlyphOverflow.Render(s.Profile)) }

// Emphasized keeps evidence at full intensity when emphasize is set, and
// demotes it otherwise — the one place that decides "is this text
// decoration or evidence" for the problem-rendering chain.
func (s Style) Emphasized(text string, emphasize bool) string {
	if emphasize {
		return text
	}
	return s.Dim(text)
}

func StateColor(s record.State) string {
	switch s {
	case record.Succeeded, record.Satisfied:
		return SGRGreen
	case record.Failed:
		return SGRRed
	case record.Refused:
		return SGRRed
	case record.Running:
		return SGRCyan
	case record.Pending, record.Excluded, record.Cancelled, record.Incomplete, record.NotStarted:
		return SGRDim
	default:
		return ""
	}
}

func EffectColor(kind string) string {
	switch kind {
	case "changed":
		return SGRGreen
	case "planned":
		return SGRBlue
	default:
		return SGRCyan
	}
}
