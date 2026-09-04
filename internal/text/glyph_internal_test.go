package text

import "testing"

// TestSpinnerFrames_ASCIIExcludesNotStartedGlyph pins evo-rec.md's "ASCII
// spinner alphabet excludes every semantic glyph" rule.
func TestSpinnerFrames_ASCIIExcludesNotStartedGlyph(t *testing.T) {
	for _, frame := range spinnerASCIIFrames {
		if frame == GlyphNotStarted.ascii || frame == "-" {
			t.Fatalf("ASCII spinner frame %q collides with Not-started's glyph", frame)
		}
	}
}

// TestGlyphSpec_CellWidth_BlockedAndCancelledAreNarrow spot-checks
// GLYPH-001's cell-width metadata for the two glyphs the work order calls
// out by name: ⊘ and ■ are one cell wide, unlike the two-cell ✓/✗.
func TestGlyphSpec_CellWidth_BlockedAndCancelledAreNarrow(t *testing.T) {
	if w := GlyphBlockedState.CellWidth(GlyphsUnicode); w != 1 {
		t.Fatalf("⊘ cell width = %d, want 1", w)
	}
	if w := GlyphCancelled.CellWidth(GlyphsUnicode); w != 1 {
		t.Fatalf("■ cell width = %d, want 1", w)
	}
	if w := GlyphDone.CellWidth(GlyphsUnicode); w != 2 {
		t.Fatalf("✓ cell width = %d, want 2", w)
	}
}
