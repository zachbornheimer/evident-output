package evo

import (
	"bytes"
	"context"
	"slices"
	"testing"
)

// abandonedWhileListenerDelayed interrupts a run with one Task queued behind a
// blocker. The graph abandons the queued Task on its own (NotStarted) while an
// open record Hold, another goroutine inside a graph section, delays the
// listener. The caller releases the Hold and the blocker.
func abandonedWhileListenerDelayed() (o *Run, queued *Task, releaseBlocker func()) {
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

	seen := slices.ContainsFunc(o.events(), func(e recEvent) bool {
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

	o.println("after settle")
	o.rec.Release()
	releaseBlocker()
	_ = o.finish()

	var order []string
	for _, e := range o.events() {
		if e.Type == "message.emitted" || (e.EntityID == queued.id && e.Type == "task.not_started") {
			order = append(order, e.Type)
		}
	}
	if len(order) != 2 || order[0] != "task.not_started" {
		t.Errorf("journal order = %v, want task.not_started then message.emitted", order)
	}
}
