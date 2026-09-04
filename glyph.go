package evo

import (
	"os"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// GlyphProfile selects which glyph vocabulary state markers render in
// (evo-rec.md "Tightened glyph vocabulary", rule GLYPH-001: glyph selection
// via capability profile, cell-width measurement not rune counts).
//
// The zero value, GlyphsAuto, keeps today's Unicode vocabulary off a TTY (a
// non-interactive stream can't show a human mojibake, so there is nothing to
// guard against) and on any TTY whose locale already advertises UTF-8. It
// downgrades to the ASCII vocabulary only on an interactive terminal without
// UTF-8 locale support — the one case where the status column would
// otherwise render as mojibake.
//
// Aliased into internal/text (glyph tables and the rendering primitives that
// select from them live there — see EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md
// §38).
type GlyphProfile = txt.GlyphProfile

const (
	// GlyphsAuto detects the vocabulary from locale and TTY interactivity.
	GlyphsAuto = txt.GlyphsAuto
	// GlyphsUnicode forces the Unicode vocabulary regardless of locale.
	GlyphsUnicode = txt.GlyphsUnicode
	// GlyphsASCII forces the ASCII vocabulary regardless of locale.
	GlyphsASCII = txt.GlyphsASCII
)

// Glyphs selects the glyph capability profile (default GlyphsAuto).
func Glyphs(p GlyphProfile) Option {
	return optionFunc(func(c *config) { c.glyphs = p })
}

// resolveGlyphProfileLocked turns a possibly-auto profile into a concrete
// one. Called once at construction (newOutput) so every render call reads a
// decided value instead of re-detecting locale per frame.
func resolveGlyphProfileLocked(cfg *config) {
	if cfg.glyphs != GlyphsAuto {
		return
	}
	if !interactiveOutputLocked(cfg) || localeAdvertisesUTF8() {
		cfg.glyphs = GlyphsUnicode
		return
	}
	cfg.glyphs = GlyphsASCII
}

// interactiveOutputLocked reports whether the configured terminal is a real
// interactive surface. Plain/non-interactive projection keeps the Unicode
// vocabulary unconditionally — evo-rec.md's glyph-safety default only
// guards the live status column a human is watching.
func interactiveOutputLocked(cfg *config) bool {
	if cfg.plain {
		return false
	}
	ls := asLive(cfg.terminal)
	return ls != nil && ls.IsInteractive()
}

// localeAdvertisesUTF8 checks LC_ALL, then LC_CTYPE, then LANG (POSIX
// override order) for a UTF-8 marker. Read directly at construction time —
// the same pattern Config.Color's NO_COLOR detection uses — so no render
// path re-reads the environment.
func localeAdvertisesUTF8() bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(name); v != "" {
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "utf8")
		}
	}
	// No locale env set at all: assume the historical default (UTF-8) rather
	// than guessing ASCII from absence.
	return true
}
