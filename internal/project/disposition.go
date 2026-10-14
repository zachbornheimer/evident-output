package project

// Disposition is which tally a Kept/Skipped record counts toward. The
// zero value is noDisposition: no tally, as when a row inlined none.
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

// Glyph is a tally's leading Glyph. A Kept tally is attention — the run
// left something in place the reader may have expected gone — so it wears
// the warning bang ("! kept 13 (...)"). A Skipped tally
// is skip detail, never a warning: it wears the plain, widely-rendered `-`
// of an already-satisfied detail (GlyphSkipDetail), and "!" is reserved for
// Warning. CONTRACT §8 folds per-item Excluded Tasks into one tally.
func (d Disposition) Glyph(s Style) string {
	if d == DispositionSkipped {
		return s.Dim(GlyphSkipDetail.Render(s.Profile))
	}
	return s.WarningGlyph()
}
