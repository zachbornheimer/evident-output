package evo

import (
	"testing"
)

// settleOutsideTheLock settles the Task named id the way a scheduler worker
// does: through the graph, with Output.mu free, while the record's
// notification is still in flight (a Hold stands for the delivery that has
// not reached the listener). It returns the release of that delivery.
func settleOutsideTheLock(o *Run, id string, state entityState) (deliver func()) {
	o.rec.Hold()
	o.graph.Settle(o.graph.Task(id), state)
	return o.rec.Release
}

// TestSnapshotFollowsASettleTheListenerHasNotHeardYet proves a read of the
// whole run answers what a settle owes before the listener hears it, and
// that the listener, hearing it later, answers it no second time.
func TestSnapshotFollowsASettleTheListenerHasNotHeardYet(t *testing.T) {
	out := newListenerTestOutput(t)
	task := out.Task("settles")
	before := out.snapshot().Version

	deliver := settleOutsideTheLock(out, task.id, failed)
	snapshot := out.snapshot()
	deliver()

	if snapshot.Version == before {
		t.Error("the snapshot's version did not move for the settle")
	}
	settledEvents := 0
	for _, e := range out.copyEvents() {
		if e.Type == "task.failed" && e.EntityID == task.id {
			settledEvents++
		}
	}
	if settledEvents != 1 {
		t.Errorf("journaled %d task.failed events, want 1", settledEvents)
	}
}
