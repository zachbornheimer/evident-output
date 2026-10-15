package evo

import (
	"strings"
	"testing"
	"time"
)

// TestPlainHeartbeat_NamesOnlyWhatStillRunsInAGroup: a Group writes as one
// block once it settles, so a member that finished early has no row yet. The
// heartbeat must not pretend otherwise: it names the member still running,
// by its own elapsed time, and stops when that member settles.
func TestPlainHeartbeat_NamesOnlyWhatStillRunsInAGroup(t *testing.T) {
	clock := newHeartbeatFakeClock()
	var buf strings.Builder
	out := initRun(Config{Name: "demo", Output: Plain, Facades: Facades{Stdout: &buf, Stderr: &buf, Clock: clock}})
	t.Cleanup(func() { _ = out.close() })

	pool := out.Group("pool")
	slow := pool.Task("slow")
	slow.Doing("working")
	fast := pool.Task("fast")
	fast.Doing("working")
	clock.Advance(10 * time.Second)
	fast.succeed("")

	clock.Advance(20 * time.Second)
	beats := heartbeatLines(buf.String())
	if len(beats) != 1 || !strings.Contains(beats[0], "pool  slow") || !strings.Contains(beats[0], "— 30s") {
		t.Fatalf("want one heartbeat, for slow alone at 30s, got %q in:\n%s", beats, buf.String())
	}

	slow.succeed("")
	before := buf.String()
	clock.Advance(2 * time.Minute)
	if buf.String() != before {
		t.Fatalf("heartbeat must stop once the group's last member settles, got:\n%s", buf.String())
	}
}

// heartbeatLines are the plain heartbeat rows in text.
func heartbeatLines(text string) []string {
	var beats []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "• ") {
			beats = append(beats, line)
		}
	}
	return beats
}
