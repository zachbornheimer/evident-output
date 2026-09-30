package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

type Printer struct{ inner *engine.Printer }

type ConfirmOption = engine.ConfirmOption
type ColorMode = engine.ColorMode
type Verbosity = engine.Verbosity
type TerminalDriver = engine.TerminalDriver
type TimeSource = engine.TimeSource
type SystemClock = engine.SystemClock
type FixedClock = engine.FixedClock
type LiveSurface = engine.LiveSurface
type Redactor = engine.Redactor
type NoopRedactor = engine.NoopRedactor

const (
	ColorAuto   = engine.ColorAuto
	ColorAlways = engine.ColorAlways
	ColorNever  = engine.ColorNever
)

const (
	VerbosityNormal  = engine.VerbosityNormal
	VerbosityVerbose = engine.VerbosityVerbose
)

const DefaultVisibleNames = engine.DefaultVisibleNames

// Visibility selects whether a message is ordinary or verbose user detail.
// Zero is VisibilityNormal.
type Visibility = core.Visibility

const (
	// VisibilityNormal messages always project at VerbosityNormal (C11:
	// prefixed consistently with VisibilityVerbose — the two enum members
	// previously disagreed on their own naming convention).
	VisibilityNormal = core.VisibilityNormal
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	VisibilityVerbose = core.VisibilityVerbose
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
