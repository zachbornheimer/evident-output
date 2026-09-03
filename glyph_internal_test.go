package evo

import "testing"

// TestTaskGlyph_ASCIITable pins the full task-state → ASCII face mapping —
// including Cancelled ([cancel]), which is only reachable in practice via a
// Confirm gate cancelled mid-prompt and is easier to pin at the table level
// than to reproduce end-to-end.
func TestTaskGlyph_ASCIITable(t *testing.T) {
	cases := map[EntityState]string{
		Done:       "[ok]",
		Failed:     "[x]",
		Blocked:    "[blocked]",
		Warning:    "[!]",
		Pending:    "[.]",
		Cancelled:  "[cancel]", // evo-rec.md: Cancelled gets its own face, no longer Pending's
		Skipped:    "[.]",
		NotStarted: "[-]",
		Incomplete: "[-]", // an unresolved task at Finish reads as "not started"
	}
	for state, want := range cases {
		if got := taskGlyph(state, GlyphsASCII); got != want {
			t.Errorf("taskGlyph(%s, ASCII) = %q, want %q", state, got, want)
		}
	}
}

// TestTaskGlyph_UnicodeUnchanged pins taskGlyph's pre-existing Unicode faces.
func TestTaskGlyph_UnicodeUnchanged(t *testing.T) {
	cases := map[EntityState]string{
		Done:       "✓",
		Failed:     "✗",
		Blocked:    "⊘",
		Warning:    "!",
		Pending:    "○",
		Cancelled:  "■", // evo-rec.md: Cancelled gets its own face, no longer Pending's
		Skipped:    "○",
		NotStarted: "-",
		Incomplete: "-", // an unresolved task at Finish reads as "not started"
	}
	for state, want := range cases {
		if got := taskGlyph(state, GlyphsUnicode); got != want {
			t.Errorf("taskGlyph(%s, Unicode) = %q, want %q", state, got, want)
		}
	}
}

// TestSpinnerFrames_ASCIIExcludesNotStartedGlyph pins evo-rec.md's "ASCII
// spinner alphabet excludes every semantic glyph" rule.
func TestSpinnerFrames_ASCIIExcludesNotStartedGlyph(t *testing.T) {
	for _, frame := range spinnerASCIIFrames {
		if frame == glyphNotStarted.ascii || frame == "-" {
			t.Fatalf("ASCII spinner frame %q collides with Not-started's glyph", frame)
		}
	}
}

// TestGlyphSpec_CellWidth_BlockedAndCancelledAreNarrow spot-checks
// GLYPH-001's cell-width metadata for the two glyphs the work order calls
// out by name: ⊘ and ■ are one cell wide, unlike the two-cell ✓/✗.
func TestGlyphSpec_CellWidth_BlockedAndCancelledAreNarrow(t *testing.T) {
	if w := glyphBlockedState.cellWidth(GlyphsUnicode); w != 1 {
		t.Fatalf("⊘ cell width = %d, want 1", w)
	}
	if w := glyphCancelled.cellWidth(GlyphsUnicode); w != 1 {
		t.Fatalf("■ cell width = %d, want 1", w)
	}
	if w := glyphDone.cellWidth(GlyphsUnicode); w != 2 {
		t.Fatalf("✓ cell width = %d, want 2", w)
	}
}

// TestResolveGlyphProfileLocked_AutoOnNonInteractiveStaysUnicode pins that a
// plain/non-interactive Output never downgrades, matching today's behavior
// regardless of locale.
func TestResolveGlyphProfileLocked_AutoOnNonInteractiveStaysUnicode(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	cfg := config{plain: true, glyphs: GlyphsAuto}
	resolveGlyphProfileLocked(&cfg)
	if cfg.glyphs != GlyphsUnicode {
		t.Fatalf("resolved profile = %v, want GlyphsUnicode", cfg.glyphs)
	}
}

// TestResolveGlyphProfileLocked_ExplicitProfileIsNeverOverridden pins that an
// explicit Glyphs() choice always wins over detection.
func TestResolveGlyphProfileLocked_ExplicitProfileIsNeverOverridden(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	cfg := config{plain: true, glyphs: GlyphsASCII}
	resolveGlyphProfileLocked(&cfg)
	if cfg.glyphs != GlyphsASCII {
		t.Fatalf("resolved profile = %v, want explicit GlyphsASCII preserved", cfg.glyphs)
	}
}
