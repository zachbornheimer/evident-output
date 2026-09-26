package render

import txt "github.com/zachbornheimer/evident-output/internal/text"

// Disposition is which tally a Kept/Skipped record counts toward. The
// zero value is NoDisposition: no tally, as when a row inlined none.
type Disposition uint8

const (
	NoDisposition Disposition = iota
	DispositionSkipped
	DispositionKept
)

// String is the verb a tally reads as ("skipped 3 (...)", "kept 2
// (...)").
func (d Disposition) String() string {
	switch d {
	case DispositionSkipped:
		return "skipped"
	case DispositionKept:
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
func (d Disposition) Glyph(s Style) string {
	if d == DispositionSkipped {
		return s.Dim(txt.GlyphSkipDetail.Render(s.Profile))
	}
	return s.warningGlyph()
}
