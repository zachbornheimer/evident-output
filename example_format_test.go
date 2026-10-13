package evo

import (
	"context"
	"fmt"
	"io"
	"time"
)

// ExampleFormat selects the overall stream-routing mode — zero is
// FormatHuman, ordinary human (and optional interactive) presentation.
func exampleFormat() {
	f := formatData
	fmt.Println(f == formatData)
	// Output:
	// true
}

// ExampleVerbosity selects which message visibilities project to the human
// stream — zero is VerbosityNormal.
func exampleVerbosity() {
	v := verbosityVerbose
	fmt.Println(v == verbosityVerbose)
	// Output:
	// true
}

// ExampleProjection selects presentation encoding independent of Format —
// human, plain, json, jsonl, or stream-json.
func exampleProjection() {
	p := projectionPlain
	fmt.Println(p == projectionPlain)
	// Output:
	// true
}

// ExampleColorMode selects the color policy — default ColorAuto uses TTY
// detection and honors NO_COLOR.
func exampleColorMode() {
	c := colorNever
	fmt.Println(c == colorNever)
	// Output:
	// true
}

// ExampleTerminalDriver shows the minimal identity interface a custom
// terminal integration implements — see Terminal.
func exampleTerminalDriver() {
	var driver terminalDriver = namedTerminal{"custom"}
	fmt.Println(driver.ID())
	// Output:
	// custom
}

// ExampleTimeSource shows the deterministic clock interface Clock injects
// in place of the real wall clock.
func exampleTimeSource() {
	var clock timeSource = fixedClock_1{T: time.Unix(0, 0).UTC()}
	fmt.Println(clock.Now())
	// Output:
	// 1970-01-01 00:00:00 +0000 UTC
}

// ExampleSystemClock uses the real wall clock — the default TimeSource.
func exampleSystemClock() {
	var clock systemClock_1
	fmt.Println(clock.Now().IsZero())
	// Output:
	// false
}

// ExampleFixedClock always returns the same instant — the deterministic
// clock every timestamp-sensitive test in this repo injects via Clock.
func exampleFixedClock() {
	clock := fixedClock_1{T: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	fmt.Println(clock.Now())
	// Output:
	// 2024-01-01 00:00:00 +0000 UTC
}

// ExampleLiveSurface shows the interactive terminal surface a live-region
// renderer paints onto — a superset of TerminalDriver.
func exampleLiveSurface() {
	var surface liveSurface = fakeLiveSurface{}
	fmt.Println(surface.ID(), surface.IsInteractive())
	// Output:
	// fake false
}

// ExampleRedactor redacts sensitive values before journal, Capture
// retention, and human rendering.
func ExampleRedactor() {
	var r Redactor = noopRedactor_1{}
	fmt.Println(r.RedactString("token=abc123"))
	// Output:
	// token=abc123
}

// ExampleNoopRedactor leaves strings unchanged — the default Redactor.
func ExampleNoopRedactor() {
	var r noopRedactor_1
	fmt.Println(r.RedactString("token=abc123"))
	// Output:
	// token=abc123
}

// ExamplePlainOptions configures RenderPlain's durable-report rendering.
func examplePlainOptions() {
	opts := plainOptions{width: 80, noColor: true}
	fmt.Println(opts.width, opts.noColor)
	// Output:
	// 80 true
}

// ExampleRenderPlain renders a Snapshot as the durable plain-text report —
// the same renderer Config.Plain wires up automatically.
func exampleRenderPlain() {
	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, plain: true, isolated: true})
	out.Task("apply patch").Define(func(context.Context) error { return nil })
	_ = out.finish()
	data, err := renderPlain_1(out.snapshot(), plainOptions{noColor: true})
	fmt.Println(err)
	fmt.Print(string(data))
	// Output:
	// <nil>
	// ✓ apply patch
}

// ExampleIsCharDevice reports whether w is an interactive character device
// (a real terminal) — false for an ordinary buffer or file.
func ExampleIsCharDevice() {
	fmt.Println(isCharDevice(io.Discard))
	// Output:
	// false
}

// ExamplePluralize formats a quantity with its singular noun pluralized
// when the quantity isn't exactly one.
func ExamplePluralize() {
	fmt.Println(pluralize(1, "branch"))
	fmt.Println(pluralize(3, "branch"))
	// Output:
	// branch
	// branches
}

// ExampleTruncateNames bounds a name list to visible entries, summarizing
// the rest.
func ExampleTruncateNames() {
	fmt.Println(truncateNames([]string{"main", "dev", "release", "hotfix"}, 2))
	// Output:
	// main, dev … +2 more
}

// namedTerminal is defined in example_option_test.go and reused here for
// ExampleTerminalDriver.

// fakeLiveSurface is a minimal LiveSurface for ExampleLiveSurface — real
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

type namedTerminal struct{ id string }

func (t namedTerminal) ID() string { return t.id }
