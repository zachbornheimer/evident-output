package engine

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// claimedTaskTrials repeats a race until a window a few instructions wide has
// been hit often enough to fail reliably when it is open.
const claimedTaskTrials = 200

// claimWithStartedHeld submits b and waits until the scheduler claimed it with
// `executing` callbacks running, while its Work.Started is held back: the
// window between a Task being claimed and its row reaching Running. The
// returned func lets Started go.
func claimWithStartedHeld(o *Output, b *TaskHandle, executing int) (release func()) {
	gate := make(chan struct{})
	o.mu.Lock()
	st := o.taskStates[b.id]
	work := o.workOf(st, func() error { return nil })
	started := work.Started
	work.Started = func() { <-gate; started() }
	o.graph.Submit(st.node, work)
	o.mu.Unlock()
	go o.graph.Kick()
	for o.graph.Executing() < executing {
		time.Sleep(50 * time.Microsecond)
	}
	return func() { close(gate) }
}

// TestInterrupt_ATaskClaimedBeforeItsRowStartedDoesNotRunToDone pins that a
// Task the scheduler claimed is Running as far as an interrupt can tell: ^C
// in the window before Work.Started took o.mu cancelled nothing, abandoned
// nothing, and the callback then settled the row Done after the interrupt.
func TestInterrupt_ATaskClaimedBeforeItsRowStartedDoesNotRunToDone(t *testing.T) {
	escaped := 0
	for range claimedTaskTrials {
		var buf bytes.Buffer
		o := newOutput("job", to(&buf), maxConcurrency(2))
		releaseA := make(chan struct{})
		o.Task("A").Define(func(context.Context) error { <-releaseA; return nil })
		b := o.Task("B")
		release := claimWithStartedHeld(o, b, 2)

		o.interrupt("x")
		release()
		close(releaseA)
		_ = o.Finish()

		if b.Snapshot().State == Done {
			escaped++
		}
	}
	if escaped > 0 {
		t.Errorf("a claimed Task ran to Done after the interrupt in %d/%d trials", escaped, claimedTaskTrials)
	}
}

// TestInterrupt_ACancelledClaimedTaskNeverAnnouncesItselfEligibleAfterFinishing
// pins that a Task cancelled between its claim and its start does not emit
// task.eligible after task.finished.
func TestInterrupt_ACancelledClaimedTaskNeverAnnouncesItselfEligibleAfterFinishing(t *testing.T) {
	late := 0
	for range claimedTaskTrials / 2 {
		var buf, stream bytes.Buffer
		o := newOutput("job", to(&buf), maxConcurrency(1), wireFormatOption(FormatJSONL), wireStreamOption(&stream))
		release := claimWithStartedHeld(o, o.Task("B"), 1)

		o.interrupt("x")
		release()
		_ = o.Finish()

		wire := stream.String()
		finished := strings.Index(wire, `"task.finished"`)
		eligible := strings.Index(wire, `"task.eligible"`)
		if finished >= 0 && eligible > finished {
			late++
		}
	}
	if late > 0 {
		t.Errorf("task.eligible followed task.finished in %d/%d trials", late, claimedTaskTrials/2)
	}
}
