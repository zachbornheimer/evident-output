package evo

import (
	"bytes"
	"strings"
	"testing"
)

// recordingLiveDriver is a minimal LiveSurface test double that records
// every call it receives, so a test can assert a debug record went through
// the clear-live -> write-durable -> repaint sequence (debugLiveLocked)
// rather than being written raw to some other writer.
type recordingLiveDriver struct {
	interactive bool
	calls       []string
}

func (d *recordingLiveDriver) ID() string          { return "recording-live" }
func (d *recordingLiveDriver) Columns() int        { return 80 }
func (d *recordingLiveDriver) Rows() int           { return 24 }
func (d *recordingLiveDriver) IsInteractive() bool { return d.interactive }
func (d *recordingLiveDriver) ClearLive()          { d.calls = append(d.calls, "clear") }
func (d *recordingLiveDriver) WriteLive(string)    {}
func (d *recordingLiveDriver) WriteDurable(s string) {
	d.calls = append(d.calls, "durable:"+s)
}
func (d *recordingLiveDriver) WriteFinal(string) {}

// TestProjectDebugRecord_DualStreamSameTerminalRoutesThroughLiveSequencing
// reproduces gate-7 finding 1: Config{Stdout, Stderr} — the realistic
// default construction — routes To(Stdout) and Diagnostics(Stderr), two
// distinct io.Writer values. On an interactive shell without redirection,
// both fds name the SAME physical tty. Before the fix, projectDebugRecordLocked's
// dual branch (output.go) wrote the debug line straight to Diagnostics with
// io.WriteString, bypassing debugLiveLocked's clear-live -> write-durable ->
// repaint sequence entirely — so the raw bytes landed mid-spinner-row on the
// one screen both writers share (observed 3/3 under a real pty: "⠏ sync
// syncing a04:06:07.997 [INFO] synced").
//
// withDiagnosticSharesTerminal stands in here for construct.go's real device
// identity detection (sameTerminalDevice), which a portable unit test cannot
// exercise without a real tty — see TestSameTerminalDevice and the pty
// verification captured in the work order report.
func TestProjectDebugRecord_DualStreamSameTerminalRoutesThroughLiveSequencing(t *testing.T) {
	drv := &recordingLiveDriver{interactive: true}
	var diagnostics, primary bytes.Buffer

	o := newOutput("gate7-finding1",
		To(&primary), Diagnostics(&diagnostics), Terminal(drv),
		withDiagnosticSharesTerminal(), DebugLevel(LevelDebug))
	t.Cleanup(func() { _ = o.Close() })

	o.Debug("dual stream debug line")

	var sawSequencedWrite bool
	for _, c := range drv.calls {
		if strings.HasPrefix(c, "durable:") && strings.Contains(c, "dual stream debug line") {
			sawSequencedWrite = true
		}
	}
	if !sawSequencedWrite {
		t.Fatalf("expected the debug line to reach the live driver via clear-live -> write-durable sequencing, got calls=%v", drv.calls)
	}
	if strings.Contains(diagnostics.String(), "dual stream debug line") {
		t.Fatalf("expected no raw duplicate write to Diagnostics when it shares the live terminal, got diagnostics=%q", diagnostics.String())
	}
}
