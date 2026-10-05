package evo

import (
	"io"

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

func wrapPrinter(inner *engine.Printer) *Printer {
	return wrap(inner, func() *Printer { return &Printer{inner: inner} })
}

func (p *Printer) impl() *engine.Printer {
	if p == nil {
		return nil
	}
	return p.inner
}

func (p *Printer) Print(args ...any) { p.impl().Print(args...) }

func (p *Printer) Printf(format string, args ...any) { p.impl().Printf(format, args...) }

func (p *Printer) Println(args ...any) { p.impl().Println(args...) }

func (p *Printer) Writer() io.Writer {
	if p == nil || p.inner == nil {
		return io.Discard
	}
	return p.inner.Writer()
}

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func Print(args ...any) { engine.Print(args...) }

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func Printf(format string, args ...any) { engine.Printf(format, args...) }

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func Println(args ...any) { engine.Println(args...) }

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func Verbose() *Printer { return wrapPrinter(engine.Verbose()) }

// Confirm asks question on the default instance and returns whether the user accepted.
func Confirm(question string, opts ...ConfirmOption) bool {
	return engine.Confirm(question, opts...)
}

func AssumeYes(v bool) ConfirmOption              { return engine.AssumeYes(v) }
func ConfirmDetail(lines ...string) ConfirmOption { return engine.ConfirmDetail(lines...) }
func Destructive() ConfirmOption                  { return engine.Destructive() }
func PolicyFlag(flag string) ConfirmOption        { return engine.PolicyFlag(flag) }
func PolicyHint(command string, args ...string) ConfirmOption {
	return engine.PolicyHint(command, args...)
}

func IsCharDevice(w io.Writer) bool { return engine.IsCharDevice(w) }
func Pluralize(quantity int64, singular string) string {
	return engine.Pluralize(quantity, singular)
}
func TruncateNames(names []string, visible int) string {
	return engine.TruncateNames(names, visible)
}

func (o *Output) Confirm(question string, opts ...ConfirmOption) bool {
	if o == nil || o.inner == nil {
		return false
	}
	return o.inner.Confirm(question, opts...)
}

func (o *Output) Print(args ...any) { o.impl().Print(args...) }

func (o *Output) Printf(format string, args ...any) { o.impl().Printf(format, args...) }

func (o *Output) Println(args ...any) { o.impl().Println(args...) }

func (o *Output) ResultWriter() io.Writer {
	if o == nil || o.inner == nil {
		return io.Discard
	}
	return o.inner.ResultWriter()
}

func (o *Output) Suspend(fn func() error) error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Suspend(fn)
}

func (o *Output) Writer() io.Writer {
	if o == nil || o.inner == nil {
		return io.Discard
	}
	return o.inner.Writer()
}
