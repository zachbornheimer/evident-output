package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// runConfig builds an isolated Output from cfg, filling Plain/Color/Stdout
// defaults that keep the rendered bytes deterministic, declares one Task,
// Finishes, and returns the rendered plain-text bytes — the shared shape
// every Config-field Example below uses to prove the field participates in
// a real run.
func runConfig(cfg evo.Config) string {
	var buf bytes.Buffer
	cfg.Isolated = true
	cfg.Plain = true
	cfg.Color = evo.ColorNever
	if cfg.Stdout == nil {
		cfg.Stdout = &buf
	}
	out := evo.Init(cfg)
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.Finish()
	return buf.String()
}

// ExampleConfig_alsoWrite mirrors every written byte to an additional
// writer — useful for tee-ing human output to a log file alongside the
// terminal.
func ExampleConfig_alsoWrite() {
	var mirror bytes.Buffer
	fmt.Print(runConfig(evo.Config{AlsoWrite: []io.Writer{&mirror}}))
	fmt.Println(mirror.Len() > 0)
	// Output:
	// ✓ demo
	// true
}

// ExampleConfig_clock injects a deterministic TimeSource in place of the
// real wall clock — the seam every timestamp-sensitive test in this repo
// uses.
func ExampleConfig_clock() {
	fixed := evo.FixedClock{T: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	fmt.Print(runConfig(evo.Config{Clock: fixed}))
	// Output:
	// ✓ demo
}

// ExampleConfig_dataFormat reserves Stdout for the application's domain
// payload; human presentation moves to Stderr.
func ExampleConfig_dataFormat() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: &buf, Format: evo.FormatData,
	})
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}

// ExampleConfig_debugAddSource resolves each debug record's call site to a
// source=file.go:line field on human/pane/history rendering.
func ExampleConfig_debugAddSource() {
	fmt.Print(runConfig(evo.Config{Debug: evo.DebugConfig{AddSource: true}}))
	// Output:
	// ✓ demo
}

// ExampleConfig_debugHistory selects durable scrollback (the default) for
// the debug journal — appended above the live region instead of a bounded
// pane.
func ExampleConfig_debugHistory() {
	fmt.Print(runConfig(evo.Config{Debug: evo.DebugConfig{View: evo.DebugPresentationHistory}}))
	// Output:
	// ✓ demo
}

// ExampleConfig_debugLevel sets the minimum debug journal level, surfacing
// Debug/Capture mirrors when set to LevelTrace or LevelDebug.
func ExampleConfig_debugLevel() {
	fmt.Print(runConfig(evo.Config{Debug: evo.DebugConfig{Level: evo.LevelDebug}}))
	// Output:
	// ✓ demo
}

// ExampleConfig_debugPane selects a bounded rolling viewport for the debug
// journal instead of durable scrollback.
func ExampleConfig_debugPane() {
	fmt.Print(runConfig(evo.Config{Debug: evo.DebugConfig{View: evo.DebugPresentationPane, PaneHeight: 3}}))
	// Output:
	// ✓ demo
}

// ExampleConfig_diagnostics routes diagnostics (the debug journal's default
// destination) to an explicit writer via Stderr.
func ExampleConfig_diagnostics() {
	var diag bytes.Buffer
	fmt.Print(runConfig(evo.Config{Stderr: &diag}))
	// Output:
	// ✓ demo
}

// ExampleConfig_dryRun declares the run a dry run: mutation verbs render as
// [planned] rows with the imperative verb instead of [changed] rows.
func ExampleConfig_dryRun() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard, DryRun: true})
	out.Task("prune branches").Define(effectOf(evo.EffectDelete, "stale branch", 3))
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// [dry-run] no changes will be made
	//
	// ✓ prune branches
	//
	// [planned] prune branches  delete 3 stale branches
	//
	// [planned]
}

// ExampleConfig_externalFormat disables inline rendering entirely — only
// snapshots are available, for an embedder that owns its own presentation.
func ExampleConfig_externalFormat() {
	fmt.Print(runConfig(evo.Config{Format: evo.FormatExternal}))
	// Output:
	// ✓ demo
}

// ExampleConfig_maxEntities caps how many entities the debug journal /
// history retains before it starts dropping the oldest.
func ExampleConfig_maxEntities() {
	fmt.Print(runConfig(evo.Config{MaxEntities: 64}))
	// Output:
	// ✓ demo
}

// ExampleConfig_maxEvents caps how many durable events the journal retains.
func ExampleConfig_maxEvents() {
	fmt.Print(runConfig(evo.Config{MaxEvents: 64}))
	// Output:
	// ✓ demo
}

// ExampleConfig_maxFrameRate caps how often the live region repaints per
// second.
func ExampleConfig_maxFrameRate() {
	fmt.Print(runConfig(evo.Config{MaxFrameRate: 30}))
	// Output:
	// ✓ demo
}

// ExampleConfig_noColor disables semantic color regardless of TTY/NO_COLOR
// detection.
func ExampleConfig_noColor() {
	fmt.Print(runConfig(evo.Config{Color: evo.ColorNever}))
	// Output:
	// ✓ demo
}

// ExampleConfig_plain disables live interactive frames, on a TTY or off —
// the durable report every non-interactive CI log needs.
func ExampleConfig_plain() {
	fmt.Print(runConfig(evo.Config{Plain: true}))
	// Output:
	// ✓ demo
}

// ExampleConfig_redactor injects a Redactor that scrubs sensitive values
// before journal, Capture retention, and human rendering.
func ExampleConfig_redactor() {
	fmt.Print(runConfig(evo.Config{Redactor: evo.NoopRedactor{}}))
	// Output:
	// ✓ demo
}

// ExampleConfig_result routes the domain-payload writer FormatData's
// ResultWriter falls back to when set.
func ExampleConfig_result() {
	var result bytes.Buffer
	fmt.Print(runConfig(evo.Config{Result: &result}))
	// Output:
	// ✓ demo
}

// ExampleConfig_stdin sets the facade Confirm reads one answer line from —
// Plain mode never reads stdin (it blocks on policy instead), so this
// Example sets Config.Terminal to keep live rendering enabled (Plain stays
// false) and only points Stdin at a canned answer.
func ExampleConfig_stdin() {
	answer := bytes.NewBufferString("y\n")
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Stdin: answer,
		Terminal: namedTerminal{"custom"},
	})
	ok := out.Confirm("continue?")
	fmt.Println(ok)
	// Output:
	// true
}

// ExampleConfig_strict makes every recorded misuse panic instead of only
// counting it — a debug-build safety net for library-authoring call sites.
func ExampleConfig_strict() {
	fmt.Print(runConfig(evo.Config{Strict: true}))
	// Output:
	// ✓ demo
}

// ExampleConfig_terminal injects a custom TerminalDriver for identity/sink
// detection instead of evident-output's own terminal probing.
func ExampleConfig_terminal() {
	fmt.Print(runConfig(evo.Config{Terminal: namedTerminal{"custom"}}))
	// Output:
	// ✓ demo
}

// ExampleTitle sets the subject shown in the conclusion band.
func ExampleTitle() {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &buf, Stderr: io.Discard, Title: "repo-retire"})
	out.Task("scan").Define(func(context.Context) error { return nil })
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ scan
	//
	// [ready]  repo-retire
}

// ExampleConfig_visibilityDelay sets the wait before the first live paint —
// use evo.Delay(d) to build the *time.Duration value from a literal.
func ExampleConfig_visibilityDelay() {
	fmt.Print(runConfig(evo.Config{VisibilityDelay: evo.Delay(0)}))
	// Output:
	// ✓ demo
}

// ExampleConfig_width sets a fixed render width instead of detecting the
// terminal's own column count.
func ExampleConfig_width() {
	fmt.Print(runConfig(evo.Config{Width: 80}))
	// Output:
	// ✓ demo
}

// ExampleConfig_glyphs selects the state-glyph vocabulary — GlyphsASCII
// forces the ASCII vocabulary regardless of locale.
func ExampleConfig_glyphs() {
	fmt.Print(runConfig(evo.Config{Glyphs: evo.GlyphsASCII}))
	// Output:
	// [ok] demo
}

// ExampleGlyphProfile shows the vocabulary selector Config.Glyphs takes: the
// zero value, GlyphsAuto, detects Unicode-vs-ASCII from locale and
// interactivity.
func ExampleGlyphProfile() {
	profile := evo.GlyphsUnicode
	fmt.Println(profile == evo.GlyphsUnicode)
	// Output:
	// true
}

// namedTerminal is a minimal evo.TerminalDriver for ExampleConfig_terminal —
// real callers pass a concrete driver (e.g. from internal/terminal); this
// package's Example only needs one that satisfies the interface.
type namedTerminal struct{ id string }

func (t namedTerminal) ID() string { return t.id }
