package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// claimedTaskTrials repeats a race until a window a few instructions wide has
// been hit often enough to fail reliably when it is open.
const claimedTaskTrials = 200

// nopCallback is a Task callback that does nothing and succeeds.
func nopCallback() error { return nil }

// claimWithStartedHeld submits b with callback run and waits until the
// scheduler claimed it with `executing` callbacks running, while its
// Work.Started is held back: the window between a Task being claimed and its
// row reaching Running. The returned func lets Started go.
func claimWithStartedHeld(o *Output, b *TaskHandle, executing int, run func() error) (release func()) {
	gate := make(chan struct{})
	o.mu.Lock()
	st := o.taskStates[b.id]
	work := o.workOf(st, run)
	started := work.Started
	work.Started = func() bool { <-gate; return started() }
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
		release := claimWithStartedHeld(o, b, 2, nopCallback)

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
		release := claimWithStartedHeld(o, o.Task("B"), 1, nopCallback)

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

// TestInterrupt_ATaskCancelledBetweenClaimAndStartNeverRunsItsCallback pins
// that a Task an interrupt settled after the scheduler claimed it, and before
// its Started step took o.mu, skips its callback: the row is Cancelled and
// never started, so the work must not happen behind it.
func TestInterrupt_ATaskCancelledBetweenClaimAndStartNeverRunsItsCallback(t *testing.T) {
	ran := 0
	for range claimedTaskTrials {
		var buf bytes.Buffer
		o := newOutput("job", to(&buf), maxConcurrency(1))
		var called atomic.Bool
		release := claimWithStartedHeld(o, o.Task("B"), 1, func() error { called.Store(true); return nil })

		o.interrupt("x")
		release()
		_ = o.Finish()

		if called.Load() {
			ran++
		}
	}
	if ran > 0 {
		t.Errorf("a cancelled-at-claim Task's callback ran in %d/%d trials", ran, claimedTaskTrials)
	}
}

// startedEventCount is how many task.started events the JSONL stream holds
// for the entity id.
func startedEventCount(stream []byte, id string) int {
	count := 0
	for line := range bytes.SplitSeq(stream, []byte("\n")) {
		if bytes.Contains(line, []byte(`"task.started"`)) && bytes.Contains(line, []byte(`"`+id+`"`)) {
			count++
		}
	}
	return count
}

// TestInterrupt_ACallbackNeverRunsWithoutATaskStartedEvent pins the pairing
// the stream promises a reader: a callback ran only for a Task that announced
// task.started, however the interrupt lands among a burst of claims.
func TestInterrupt_ACallbackNeverRunsWithoutATaskStartedEvent(t *testing.T) {
	const tasksPerTrial = 40
	unannounced := 0
	for range claimedTaskTrials {
		var buf, stream bytes.Buffer
		o := newOutput("job", to(&buf), maxConcurrency(4), wireFormatOption(FormatJSONL), wireStreamOption(&stream))
		var mu sync.Mutex
		ran := map[string]bool{}
		handles := make([]*TaskHandle, 0, tasksPerTrial)
		for i := range tasksPerTrial {
			h := o.Task(fmt.Sprintf("t%d", i))
			handles = append(handles, h)
			h.Define(func(context.Context) error { mu.Lock(); ran[h.id] = true; mu.Unlock(); return nil })
		}

		o.interrupt("x")
		_ = o.Finish()

		for _, h := range handles {
			mu.Lock()
			didRun := ran[h.id]
			mu.Unlock()
			if didRun && startedEventCount(stream.Bytes(), h.id) == 0 {
				unannounced++
			}
		}
	}
	if unannounced > 0 {
		t.Errorf("%d callbacks ran with no task.started event", unannounced)
	}
}
