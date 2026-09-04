package evo

import (
	"bytes"
	"io"
	"testing"
)

// fakeSinkTerminal is a minimal LiveSurface + sinkReporter test double
// standing in for a caller-supplied TerminalDriver (the examples/terminal-driver
// shape: a custom driver wrapping the same writer as Config.Stdout).
type fakeSinkTerminal struct {
	w io.Writer
}

func (f *fakeSinkTerminal) ID() string          { return "fake-sink" }
func (f *fakeSinkTerminal) Columns() int        { return 80 }
func (f *fakeSinkTerminal) Rows() int           { return 24 }
func (f *fakeSinkTerminal) IsInteractive() bool { return true }
func (f *fakeSinkTerminal) ClearLive()          {}
func (f *fakeSinkTerminal) WriteLive(string)    {}
func (f *fakeSinkTerminal) WriteDurable(string) {}
func (f *fakeSinkTerminal) WriteFinal(string)   {}
func (f *fakeSinkTerminal) Sink() io.Writer     { return f.w }

// TestConfigToOptions_CallerTerminalSharingPrimaryIsDetected proves
// configToOptions DETECTS a caller-supplied Terminal(...) (Config.Terminal
// or the Options path) sharing a stream with primary via the driver's own
// Sink(), instead of only knowing that for the one construction path that
// builds both the ANSI driver and primary itself (X3). Before the fix, this
// path never set samePrimaryAsTerminal, so Finish dual-wrote the conclusion
// band onto the same physical screen the driver had already painted it on
// (the examples/terminal-driver double-band bug).
func TestConfigToOptions_CallerTerminalSharingPrimaryIsDetected(t *testing.T) {
	var buf bytes.Buffer
	term := &fakeSinkTerminal{w: &buf}

	cfg := resolveConfig(Config{Stdout: &buf, Stderr: &bytes.Buffer{}, Terminal: term})
	opts := configToOptions(cfg)

	var built config
	for _, o := range opts {
		o.apply(&built)
	}
	if !built.samePrimaryAsTerminal {
		t.Fatal("expected samePrimaryAsTerminal to be detected via the driver's Sink() matching primary")
	}
}
