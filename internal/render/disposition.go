package render

import txt "github.com/zachbornheimer/evident-output/internal/text"

// disposition is which tally a Kept/Skipped record counts toward. The
// zero value is noDisposition: no tally, as when a row inlined none.
type disposition uint8

const (
	noDisposition disposition = iota
	dispositionSkipped
	dispositionKept
)

// String is the verb a tally reads as ("skipped 3 (...)", "kept 2
// (...)").
func (d disposition) String() string {
	switch d {
	case dispositionSkipped:
		return "skipped"
	case dispositionKept:
		return "kept"
	default:
		return ""
	}
}

// glyph is a tally's leading glyph. A Kept tally is attention — the run
// left something in place the reader may have expected gone — so it wears
// the warning bang (contract §26/§27: "! kept 13 (...)"). A Skipped tally
// is skip detail, never a warning: contract §20 "Use a plain,
// widely-rendered `-` for an already-satisfied/skipped detail", and §41
// reserves "!" for Warning.
func (d disposition) glyph(color bool, profile txt.GlyphProfile) string {
	if d == dispositionSkipped {
		return txt.Dim(txt.GlyphSkipDetail.Render(profile), color)
	}
	return txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
}
