package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestLiveCensusMatchesAWalk pins the invariant liveCensus exists for: a
// collection's census is exactly what walking every Task below it counts,
// through declaration, Running, every terminal state, a warning, and a
// live paint (earliestSeen aside: a walk cannot see when a finished Task
// was stamped).
func TestLiveCensusMatchesAWalk(t *testing.T) {
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(&countingLiveSurface{}), visibilityDelay(0), withClock(clock), withNoColor(), maxConcurrency(2))
	t.Cleanup(func() { _ = out.Close() })

	root := out.Group("root")
	a, b := root.Group("a"), root.Group("b").Group("deep")
	a.Task("ok").Define(func(context.Context) error { return nil })
	a.Task("bad").Define(func(context.Context) error { return errors.New("boom") })
	warned := b.Task("warned")
	warned.Define(func(context.Context) error {
		warned.Problem("careful", Severity(SeverityWarning))
		return nil
	})
	skipped := b.Task("skipped")
	skipped.Define(func(context.Context) error { skipped.Skipped(Reason("n/a")); return nil })
	_ = root.Wait()
	release := make(chan struct{})
	running := make(chan struct{})
	b.Task("running").Define(func(context.Context) error { close(running); <-release; return nil })
	a.Task("pending")
	<-running
	t.Cleanup(func() { close(release) })

	out.mu.Lock()
	defer out.mu.Unlock()
	_ = out.liveSnapshotLocked(24, clock.t)
	for _, col := range out.containerStates {
		var want liveCensus
		walkCensus(col, &want)
		got := col.census
		want.earliestSeen = got.earliestSeen
		if got != want {
			t.Errorf("%s: census = %+v, a walk counts %+v", col.name, got, want)
		}
	}
}

// walkCensus counts every Task at or below col the slow way.
func walkCensus(col *tasksState, c *liveCensus) {
	for _, st := range col.tasks {
		c.total++
		c.count(st.rec.State(), 1)
		if len(st.rec.Warnings()) > 0 {
			c.warned++
		}
		if st.unstampedIn(st.rec.State()) {
			c.unstamped++
		}
		c.nameWidth = max(c.nameWidth, len([]rune(st.name)))
	}
	for _, child := range col.children {
		walkCensus(child, c)
	}
}
