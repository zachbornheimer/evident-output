package evo_test

import (
	"fmt"
	"io"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleFormat selects the overall stream-routing mode — zero is
// FormatHuman, ordinary human (and optional interactive) presentation.
func ExampleFormat() {
	f := evo.FormatData
	fmt.Println(f == evo.FormatData)
	// Output:
	// true
}

// ExampleVerbosity selects which message visibilities project to the human
// stream — zero is VerbosityNormal.
func ExampleVerbosity() {
	v := evo.VerbosityVerbose
	fmt.Println(v == evo.VerbosityVerbose)
	// Output:
	// true
}

// ExampleProjection selects presentation encoding independent of Format —
// human, plain, json, jsonl, or stream-json.
func ExampleProjection() {
	p := evo.ProjectionPlain
	fmt.Println(p == evo.ProjectionPlain)
	// Output:
	// true
}

// ExampleColorMode selects the color policy — default ColorAuto uses TTY
// detection and honors NO_COLOR.
func ExampleColorMode() {
	c := evo.ColorNever
	fmt.Println(c == evo.ColorNever)
	// Output:
	// true
}

// ExampleTerminalDriver shows the minimal identity interface a custom
// terminal integration implements — see evo.Terminal.
func ExampleTerminalDriver() {
	var driver evo.TerminalDriver = namedTerminal{"custom"}
	fmt.Println(driver.ID())
	// Output:
	// custom
}

// ExampleTimeSource shows the deterministic clock interface Clock injects
// in place of the real wall clock.
func ExampleTimeSource() {
	var clock evo.TimeSource = evo.FixedClock{T: time.Unix(0, 0).UTC()}
	fmt.Println(clock.Now())
	// Output:
	// 1970-01-01 00:00:00 +0000 UTC
}

// ExampleSystemClock uses the real wall clock — the default TimeSource.
func ExampleSystemClock() {
	var clock evo.SystemClock
	fmt.Println(clock.Now().IsZero())
	// Output:
	// false
}

// ExampleFixedClock always returns the same instant — the deterministic
// clock every timestamp-sensitive test in this repo injects via evo.Clock.
func ExampleFixedClock() {
	clock := evo.FixedClock{T: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	fmt.Println(clock.Now())
	// Output:
	// 2024-01-01 00:00:00 +0000 UTC
}

// ExampleLiveSurface shows the interactive terminal surface a live-region
// renderer paints onto — a superset of TerminalDriver.
func ExampleLiveSurface() {
	var surface evo.LiveSurface = fakeLiveSurface{}
	fmt.Println(surface.ID(), surface.IsInteractive())
	// Output:
	// fake false
}

// ExampleRedactor redacts sensitive values before journal, Capture
// retention, and human rendering.
func ExampleRedactor() {
	var r evo.Redactor = evo.NoopRedactor{}
	fmt.Println(r.RedactString("token=abc123"))
	// Output:
	// token=abc123
}

// ExampleNoopRedactor leaves strings unchanged — the default Redactor.
func ExampleNoopRedactor() {
	var r evo.NoopRedactor
	fmt.Println(r.RedactString("token=abc123"))
	// Output:
	// token=abc123
}

// ExamplePlainOptions configures RenderPlain's durable-report rendering.
func ExamplePlainOptions() {
	opts := evo.PlainOptions{Width: 80, NoColor: true}
	fmt.Println(opts.Width, opts.NoColor)
	// Output:
	// 80 true
}

// ExampleRenderPlain renders a Snapshot as the durable plain-text report —
// the same renderer Config.Plain wires up automatically.
func ExampleRenderPlain() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("apply patch").Done()
	_ = out.Finish()
	data, err := evo.RenderPlain(out.Snapshot(), evo.PlainOptions{NoColor: true})
	fmt.Println(err)
	fmt.Print(string(data))
	// Output:
	// <nil>
	// ✓ apply patch
}

// ExampleIsCharDevice reports whether w is an interactive character device
// (a real terminal) — false for an ordinary buffer or file.
func ExampleIsCharDevice() {
	fmt.Println(evo.IsCharDevice(io.Discard))
	// Output:
	// false
}

// ExamplePluralize formats a quantity with its singular noun pluralized
// when the quantity isn't exactly one.
func ExamplePluralize() {
	fmt.Println(evo.Pluralize(1, "branch"))
	fmt.Println(evo.Pluralize(3, "branch"))
	// Output:
	// branch
	// branches
}

// ExampleTruncateNames bounds a name list to visible entries, summarizing
// the rest.
func ExampleTruncateNames() {
	fmt.Println(evo.TruncateNames([]string{"main", "dev", "release", "hotfix"}, 2))
	// Output:
	// main, dev … +2 more
}

// namedTerminal is defined in example_option_test.go and reused here for
// ExampleTerminalDriver.

// fakeLiveSurface is a minimal evo.LiveSurface for ExampleLiveSurface — real
// callers pass a concrete driver (e.g. from internal/terminal).
type fakeLiveSurface struct{}

func (fakeLiveSurface) ID() string          { return "fake" }
func (fakeLiveSurface) Columns() int        { return 80 }
func (fakeLiveSurface) Rows() int           { return 24 }
func (fakeLiveSurface) IsInteractive() bool { return false }
func (fakeLiveSurface) WriteLive(string)    {}
func (fakeLiveSurface) ClearLive()          {}
func (fakeLiveSurface) WriteDurable(string) {}
func (fakeLiveSurface) WriteFinal(string)   {}
