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

// TestProjectDebugRecord_SameTerminalRoutesThroughLiveSequencing reproduces
// gate-7 finding 1: rows and debug records share stderr with the live region.
// A debug line written raw to stderr would land mid-spinner-row (observed 3/3
// under a real pty: "⠏ sync syncing a04:06:07.997 [INFO] synced"), so it must
// go through the clear-live -> write-durable -> repaint sequence, and only there.
func TestProjectDebugRecord_SameTerminalRoutesThroughLiveSequencing(t *testing.T) {
	drv := &recordingLiveDriver{interactive: true}
	var stderr bytes.Buffer

	o := newOutput("gate7-finding1", to(&stderr), withTerminal(drv), debugLevel(levelDebug))
	t.Cleanup(func() { _ = o.close() })

	o.debug("same terminal debug line")

	var sawSequencedWrite bool
	for _, c := range drv.calls {
		if strings.HasPrefix(c, "durable:") && strings.Contains(c, "same terminal debug line") {
			sawSequencedWrite = true
		}
	}
	if !sawSequencedWrite {
		t.Fatalf("expected the debug line to reach the live driver via clear-live -> write-durable sequencing, got calls=%v", drv.calls)
	}
	if strings.Contains(stderr.String(), "same terminal debug line") {
		t.Fatalf("expected no raw write to stderr beside the live region, got stderr=%q", stderr.String())
	}
}
