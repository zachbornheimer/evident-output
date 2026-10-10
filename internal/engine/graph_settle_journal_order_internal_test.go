package engine

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// settleJournalTrials repeats a race until a narrow window has been hit often
// enough to fail reliably when it is open.
const settleJournalTrials = 200

// queuedTaskCount is how many Tasks sit behind the blocker, enough that the
// interrupt's sweep takes a moment to reach the awaited one.
const queuedTaskCount = 300

// TestGraphSettle_AWaiterThatSawTheSettleFindsItsEventInTheJournal pins that
// a Wait returning means the Task's task.<state> event is readable: a settle
// the graph made on its own (an interrupt abandoning the queue) reaches the
// journal before the Wait that observed it returns.
func TestGraphSettle_AWaiterThatSawTheSettleFindsItsEventInTheJournal(t *testing.T) {
	missing := 0
	for range settleJournalTrials {
		var buf bytes.Buffer
		o := newOutput("job", to(&buf), maxConcurrency(1))
		release := make(chan struct{})
		o.Task("blocker").Define(func(context.Context) error { <-release; return nil })
		var queued []*TaskHandle
		for i := range queuedTaskCount {
			queued = append(queued, o.Task(fmt.Sprintf("q%d", i)).Define(func(context.Context) error { return nil }))
		}
		target := queued[0]
		found := make(chan bool)
		go func() {
			_ = target.Wait()
			found <- slices.ContainsFunc(o.Events(), func(e Event) bool {
				return e.EntityID == target.id && e.Type == "task.not_started"
			})
		}()
		for o.graph.Waits() == 0 {
			time.Sleep(time.Millisecond)
		}
		o.interrupt("x")
		if !<-found {
			missing++
		}
		close(release)
		_ = o.Finish()
	}
	if missing > 0 {
		t.Errorf("task.not_started was missing from Events() after Wait returned in %d/%d trials", missing, settleJournalTrials)
	}
}

// abandonedWhileListenerDelayed interrupts a run with one Task queued behind a
// blocker. The graph abandons the queued Task on its own (NotStarted) while an
// open record Hold, another goroutine inside a graph section, delays the
// listener. The caller releases the Hold and the blocker.
func abandonedWhileListenerDelayed() (o *Output, queued *TaskHandle, releaseBlocker func()) {
	var buf bytes.Buffer
	o = newOutput("job", to(&buf), maxConcurrency(1))
	release := make(chan struct{})
	o.Task("blocker").Define(func(context.Context) error { <-release; return nil })
	queued = o.Task("queued").Define(func(context.Context) error { return nil })
	o.rec.Hold()
	o.interrupt("x")
	return o, queued, func() { close(release) }
}

// TestGraphSettle_EventsShowASettleTheListenerHasNotDelivered pins that
// reading the journal first follows the record: a settle made inside the
// graph is visible even before the listener has told the engine about it.
func TestGraphSettle_EventsShowASettleTheListenerHasNotDelivered(t *testing.T) {
	o, queued, releaseBlocker := abandonedWhileListenerDelayed()
	defer releaseBlocker()
	defer o.rec.Release()

	seen := slices.ContainsFunc(o.Events(), func(e Event) bool {
		return e.EntityID == queued.id && e.Type == "task.not_started"
	})

	if !seen {
		t.Error("Events() did not show task.not_started for an abandoned Task")
	}
}

// TestGraphSettle_AnOutputWrittenAfterAnInternalSettleIsJournaledAfterIt pins
// that a settle the graph made on its own is journaled before what the same
// goroutine writes next, even while the listener is delayed.
func TestGraphSettle_AnOutputWrittenAfterAnInternalSettleIsJournaledAfterIt(t *testing.T) {
	o, queued, releaseBlocker := abandonedWhileListenerDelayed()

	o.Println("after settle")
	o.rec.Release()
	releaseBlocker()
	_ = o.Finish()

	var order []string
	for _, e := range o.Events() {
		if e.Type == "message.emitted" || (e.EntityID == queued.id && e.Type == "task.not_started") {
			order = append(order, e.Type)
		}
	}
	if len(order) != 2 || order[0] != "task.not_started" {
		t.Errorf("journal order = %v, want task.not_started then message.emitted", order)
	}
}
