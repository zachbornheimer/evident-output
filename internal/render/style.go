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

// dim demotes subordinate text.
func (s Style) dim(text string) string { return txt.Dim(text, s.Color) }

// paint applies an SGR code to text.
func (s Style) paint(text, sgr string) string { return txt.Style(text, sgr, s.Color) }

// stateGlyph is a Task or container state's glyph in its state color.
func (s Style) stateGlyph(state core.EntityState) string {
	return txt.StyleGlyph(TaskGlyph(state, s.Profile), StateColor(state), s.Color)
}

// warningGlyph is the yellow attention bang ("!").
func (s Style) warningGlyph() string {
	return txt.StyleGlyph(txt.GlyphWarningState.Render(s.Profile), txt.SGRYellow, s.Color)
}

// evidenceGlyph is the dim evidence connector ("└─").
func (s Style) evidenceGlyph() string { return s.dim(txt.GlyphEvidence.Render(s.Profile)) }

// overflowGlyph is the dim omission marker ("…").
func (s Style) overflowGlyph() string { return s.dim(txt.GlyphOverflow.Render(s.Profile)) }

// emphasized keeps evidence at full intensity when emphasize is set, and
// demotes it otherwise — the one place that decides "is this text
// decoration or evidence" for the problem-rendering chain.
func (s Style) emphasized(text string, emphasize bool) string {
	if emphasize {
		return text
	}
	return s.dim(text)
}
