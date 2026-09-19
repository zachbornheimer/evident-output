package evo_test

import (
	"bytes"
	"fmt"
	"io"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// runOption builds an isolated Output from the raw-Option escape hatch
// (Config.Options), declares one Task, Finishes, and returns the rendered
// plain-text bytes — the shared shape every Option Example below uses to
// prove its option compiles and participates in a real run.
func runOption(opts ...evo.Option) string {
	var buf bytes.Buffer
	all := append([]evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}, opts...)
	out := evo.Init(evo.Config{Isolated: true, Options: all})
	out.Task("demo").Done()
	_ = out.Finish()
	return buf.String()
}

// ExampleOption shows the functional-option interface every Config knob
// below (AlsoWrite, Plain, Width, ...) implements.
func ExampleOption() {
	opt := evo.NoColor()
	fmt.Println(opt != nil)
	// Output:
	// true
}

// ExampleTo routes the ordinary human stream to an explicit writer.
func ExampleTo() {
	fmt.Print(runOption())
	// Output:
	// ✓ demo
}

// ExampleAlsoWrite mirrors every written byte to an additional writer —
// useful for tee-ing human output to a log file alongside the terminal.
func ExampleAlsoWrite() {
	var mirror bytes.Buffer
	fmt.Print(runOption(evo.AlsoWrite(&mirror)))
	fmt.Println(mirror.Len() > 0)
	// Output:
	// ✓ demo
	// true
}

// ExampleClock injects a deterministic TimeSource in place of the real wall
// clock — the seam every timestamp-sensitive test in this repo uses.
func ExampleClock() {
	fixed := evo.FixedClock{T: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	fmt.Print(runOption(evo.Clock(fixed)))
	// Output:
	// ✓ demo
}

// ExampleDataProjection reserves Stdout for the application's domain
// payload; human presentation moves to Stderr.
func ExampleDataProjection() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: &buf, Options: []evo.Option{evo.DataProjection()},
	})
	out.Task("demo").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExampleDebugAddSource resolves each debug record's call site to a
// source=file.go:line field on human/pane/history rendering.
func ExampleDebugAddSource() {
	fmt.Print(runOption(evo.DebugAddSource()))
	// Output:
	// ✓ demo
}

// ExampleDebugHistory selects durable scrollback (the default) for the
// debug journal — appended above the live region instead of a bounded pane.
func ExampleDebugHistory() {
	fmt.Print(runOption(evo.DebugHistory()))
	// Output:
	// ✓ demo
}

// ExampleDebugLevel sets the minimum debug journal level, surfacing
// Debug/Capture mirrors when set to LevelTrace or LevelDebug.
func ExampleDebugLevel() {
	fmt.Print(runOption(evo.DebugLevel(evo.LevelDebug)))
	// Output:
	// ✓ demo
}

// ExampleDebugPane selects a bounded rolling viewport for the debug journal
// instead of durable scrollback.
func ExampleDebugPane() {
	fmt.Print(runOption(evo.DebugPane(evo.PaneHeight(3))))
	// Output:
	// ✓ demo
}

// ExampleDiagnostics routes diagnostics (the debug journal's default
// destination) to an explicit writer.
func ExampleDiagnostics() {
	var diag bytes.Buffer
	fmt.Print(runOption(evo.Diagnostics(&diag)))
	// Output:
	// ✓ demo
}

// ExampleDryRun declares the run a dry run: mutation verbs render as
// [planned] rows with the imperative verb instead of [changed] rows.
func ExampleDryRun() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard, DryRun: true})
	out.Task("prune branches").Delete("stale branch", func() error { return nil }, evo.Affected(3))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// [dry-run] no changes will be made
	//
	// ✓ prune branches
	//
	// [planned] prune branches  delete 3 stale branches
}

// ExampleExternalProjection disables inline rendering entirely — only
// snapshots are available, for an embedder that owns its own presentation.
func ExampleExternalProjection() {
	fmt.Print(runOption(evo.ExternalProjection()))
	// Output:
	// ✓ demo
}

// ExampleMaxEntities caps how many entities the debug journal / history
// retains before it starts dropping the oldest.
func ExampleMaxEntities() {
	fmt.Print(runOption(evo.MaxEntities(64)))
	// Output:
	// ✓ demo
}

// ExampleMaxEvents caps how many durable events the journal retains.
func ExampleMaxEvents() {
	fmt.Print(runOption(evo.MaxEvents(64)))
	// Output:
	// ✓ demo
}

// ExampleMaxFrameRate caps how often the live region repaints per second.
func ExampleMaxFrameRate() {
	fmt.Print(runOption(evo.MaxFrameRate(30)))
	// Output:
	// ✓ demo
}

// ExampleNoColor disables semantic color regardless of TTY/NO_COLOR
// detection.
func ExampleNoColor() {
	fmt.Print(runOption())
	// Output:
	// ✓ demo
}

// ExamplePlain disables live interactive frames, on a TTY or off — the
// durable report every non-interactive CI log needs.
func ExamplePlain() {
	fmt.Print(runOption())
	// Output:
	// ✓ demo
}

// ExampleRedact injects a Redactor that scrubs sensitive values before
// journal, Capture retention, and human rendering.
func ExampleRedact() {
	fmt.Print(runOption(evo.Redact(evo.NoopRedactor{})))
	// Output:
	// ✓ demo
}

// ExampleResultStream routes the domain-payload writer FormatData's
// ResultWriter falls back to when Config.Result is unset.
func ExampleResultStream() {
	var result bytes.Buffer
	fmt.Print(runOption(evo.ResultStream(&result)))
	// Output:
	// ✓ demo
}

// ExampleStdin sets the facade Confirm reads one answer line from — Plain
// mode never reads stdin (it blocks on policy instead), so this Example
// leaves live rendering enabled and only points Stdin at a canned answer.
func ExampleStdin() {
	answer := bytes.NewBufferString("y\n")
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: io.Discard, Stderr: io.Discard,
		Options: []evo.Option{evo.To(io.Discard), evo.Stdin(answer)},
	})
	ok := out.Confirm("continue?")
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleStrict makes every recorded misuse panic instead of only counting
// it — a debug-build safety net for library-authoring call sites.
func ExampleStrict() {
	fmt.Print(runOption(evo.Strict()))
	// Output:
	// ✓ demo
}

// ExampleTerminal injects a custom TerminalDriver for identity/sink
// detection instead of evident-output's own terminal probing.
func ExampleTerminal() {
	fmt.Print(runOption(evo.Terminal(namedTerminal{"custom"})))
	// Output:
	// ✓ demo
}

// ExampleTitle sets the subject shown in the conclusion band.
func ExampleTitle() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard, Title: "repo-retire"})
	out.Task("scan").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ scan
	//
	// [ready]  repo-retire
}

// ExampleVisibilityDelay sets the wait before the first live paint — use
// evo.Delay(d) to build the *time.Duration value from a literal.
func ExampleVisibilityDelay() {
	fmt.Print(runOption(evo.VisibilityDelay(0)))
	// Output:
	// ✓ demo
}

// ExampleWidth sets a fixed render width instead of detecting the
// terminal's own column count.
func ExampleWidth() {
	fmt.Print(runOption(evo.Width(80)))
	// Output:
	// ✓ demo
}

// ExampleGlyphs selects the state-glyph vocabulary — GlyphsASCII forces the
// ASCII vocabulary regardless of locale.
func ExampleGlyphs() {
	fmt.Print(runOption(evo.Glyphs(evo.GlyphsASCII)))
	// Output:
	// [ok] demo
}

// ExampleGlyphProfile shows the vocabulary selector Glyphs takes: the zero
// value, GlyphsAuto, detects Unicode-vs-ASCII from locale and interactivity.
func ExampleGlyphProfile() {
	profile := evo.GlyphsUnicode
	fmt.Println(profile == evo.GlyphsUnicode)
	// Output:
	// true
}

// namedTerminal is a minimal evo.TerminalDriver for ExampleTerminal — real
// callers pass a concrete driver (e.g. from internal/terminal); this
// package's Example only needs one that satisfies the interface.
type namedTerminal struct{ id string }

func (t namedTerminal) ID() string { return t.id }
