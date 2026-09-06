package evo

import (
	"github.com/zachbornheimer/evident-output/internal/engine"
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
func Glyphs(p GlyphProfile) Option { return engine.Glyphs(p) }
