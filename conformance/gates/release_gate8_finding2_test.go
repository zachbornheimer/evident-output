package gates_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// sinkTerminal is a minimal, non-live TerminalDriver that reports its own
// destination writer via Sink() io.Writer — the same shape real
// terminal.ANSI exposes when piped to a non-TTY.
type sinkTerminal struct{ w io.Writer }

func (t sinkTerminal) ID() string      { return "fake-sink" }
func (t sinkTerminal) Sink() io.Writer { return t.w }

// opaqueTerminal is a TerminalDriver that cannot report a destination at
// all (no Sink() method) — the driver evident-output cannot safely default
// a primary writer for.
type opaqueTerminal struct{}

func (opaqueTerminal) ID() string { return "opaque" }

// TestTerminalOption_DefaultsPrimaryToDriverSink is the green branch of
// release-gate round 8 finding 2: a Terminal driver supplied without To()
// but able to report its own Sink() must have its residual/plain projection
// land there instead of silently disappearing at exit 0.
func TestTerminalOption_DefaultsPrimaryToDriverSink(t *testing.T) {
	var sink bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		Terminal: sinkTerminal{w: &sink}, Plain: true, Color: evo.ColorNever})

	out.Task("branches").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	if sink.Len() == 0 {
		t.Fatal("Terminal's own Sink() must receive the residual projection when To() is unset — got zero output")
	}
	if !strings.Contains(sink.String(), "branches") {
		t.Fatalf("want task rendered on the driver's sink, got:\n%s", sink.String())
	}
}

// TestTerminalOption_OpaqueDriverStillRendersOnStdout is the Config-era
// form of the old "no To() + no Sink()" case. Init always fills Stdout
// (os.Stdout when unset), so an opaque driver cannot strand the residual
// projection: an explicit Stdout still receives the task row.
func TestTerminalOption_OpaqueDriverStillRendersOnStdout(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		Stdout:   &buf,
		Terminal: opaqueTerminal{},
		Plain:    true,
		Color:    evo.ColorNever,
	})

	out.Task("branches").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil when Stdout is configured", err)
	}
	if !strings.Contains(buf.String(), "branches") {
		t.Fatalf("want the task rendered on Stdout, got:\n%s", buf.String())
	}
}
