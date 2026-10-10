package engine

import (
	"testing"
	"time"
)

// settleOutsideTheLock settles the Task named id the way a scheduler worker
// does: through the graph, with Output.mu free, while the record's
// notification is still in flight (a Hold stands for the delivery that has
// not reached the listener). It returns the release of that delivery.
func settleOutsideTheLock(o *Output, id string, state EntityState) (deliver func()) {
	o.rec.Hold()
	o.graph.Settle(o.graph.Task(id), state)
	return o.rec.Release
}

// TestLiveIndexFollowsASettleTheListenerHasNotHeardYet proves a read of the
// live index finds a Task that settled outside Output.mu before the listener
// was told: the census and the filing of the Task's Group read the record,
// not the notification. liveIndexAudit fails the frame if the index differs
// from a walk of the Group's children.
func TestLiveIndexFollowsASettleTheListenerHasNotHeardYet(t *testing.T) {
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(&countingLiveSurface{}), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	items := out.Group("items")
	first, _ := items.Task("first"), items.Task("second")

	deliver := settleOutsideTheLock(out, first.id, Done)

	out.mu.Lock()
	_ = out.liveSnapshotLocked(24, clock.t)
	col := out.containerStates[items.id]
	var want liveCensus
	walkCensus(col, &want)
	got := *col.currentCensus()
	want.earliestSeen = got.earliestSeen
	out.mu.Unlock()
	deliver()

	if got != want {
		t.Errorf("census = %+v, a walk counts %+v", got, want)
	}
	if got.done != 1 {
		t.Errorf("census counts %d Done, want the settled Task counted", got.done)
	}
}

// TestSnapshotFollowsASettleTheListenerHasNotHeardYet proves a read of the
// whole run answers what a settle owes before the listener hears it, and
// that the listener, hearing it later, answers it no second time.
func TestSnapshotFollowsASettleTheListenerHasNotHeardYet(t *testing.T) {
	out := newListenerTestOutput(t)
	task := out.Task("settles")
	before := out.Snapshot().Version

	deliver := settleOutsideTheLock(out, task.id, Failed)
	snapshot := out.Snapshot()
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
