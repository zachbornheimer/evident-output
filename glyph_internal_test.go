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
