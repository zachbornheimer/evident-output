package evo

import "testing"

// TestResolveGlyphProfileLocked_AutoOnNonInteractiveStaysUnicode pins that a
// plain/non-interactive Output never downgrades, matching today's behavior
// regardless of locale.
func TestResolveGlyphProfileLocked_AutoOnNonInteractiveStaysUnicode(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	cfg := config{plain: true, glyphs: glyphsAuto}
	resolveGlyphProfileLocked(&cfg)
	if cfg.glyphs != glyphsUnicode {
		t.Fatalf("resolved profile = %v, want GlyphsUnicode", cfg.glyphs)
	}
}

// TestResolveGlyphProfileLocked_ExplicitProfileIsNeverOverridden pins that an
// explicit glyphs() choice always wins over detection.
func TestResolveGlyphProfileLocked_ExplicitProfileIsNeverOverridden(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	cfg := config{plain: true, glyphs: glyphsASCII}
	resolveGlyphProfileLocked(&cfg)
	if cfg.glyphs != glyphsASCII {
		t.Fatalf("resolved profile = %v, want explicit GlyphsASCII preserved", cfg.glyphs)
	}
}
