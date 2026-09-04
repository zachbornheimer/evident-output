package evo

import "github.com/zachbornheimer/evident-output/internal/render"

// PlainOptions configures pure plain projection (§25.4).
type PlainOptions struct {
	Width   int
	NoColor bool
	// Verbose additionally emits per-reason name lists under a task's skip/keep
	// taxonomy line. Counts and the reason partition always render; Verbose
	// only adds the bounded (TruncateNames) name detail.
	Verbose bool
	// Glyphs selects the state-glyph vocabulary. Plain projection has no live
	// TTY to detect, so GlyphsAuto (the default) resolves to GlyphsUnicode —
	// callers rendering off a known non-UTF-8 destination pass GlyphsASCII.
	Glyphs GlyphProfile
}

// RenderPlain projects a snapshot to plain text without terminal ownership.
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	glyphs := opts.Glyphs
	if glyphs == GlyphsAuto {
		glyphs = GlyphsUnicode
	}
	return []byte(render.Plain(s, opts.Width, opts.NoColor, opts.Verbose, glyphs)), nil
}
