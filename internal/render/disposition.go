package render

import txt "github.com/zachbornheimer/evident-output/internal/text"

// skippedTaxonomyGlyph is a Skipped tally's leading glyph. A Skipped tally
// is skip detail, never a warning: contract §20 "Use a plain, widely-
// rendered `-` for an already-satisfied/skipped detail", §13 "Skipped uses
// the `-` glyph and never sets warned", and §41 reserves "!" for Warning.
// Skipped is the only disposition (contract Vocabulary), so this glyph is
// shared by every taxonomy row rather than selected per-verb.
func skippedTaxonomyGlyph(s Style) string {
	return s.dim(txt.GlyphSkipDetail.Render(s.Profile))
}
