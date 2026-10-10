package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

// nestedStepCost is what a Sequence of nested steps costs to declare and
// run: the predecessors its Tasks hold once declared, and the predecessor
// outcomes scheduling read.
type nestedStepCost struct {
	preds, predChecks int
}

// scheduleNestedSteps declares a Sequence of n nested Groups, one Task
// each (a Sequence of phase Groups), Defines every Task, and waits for the
// Sequence.
func scheduleNestedSteps(tb testing.TB, n int) nestedStepCost {
	tb.Helper()
	out := Init(Config{Isolated: true, StateDir: tb.TempDir(), Stdout: io.Discard, Stderr: io.Discard})
	defer func() { _ = out.Close() }()
	seq := out.Sequence("phases")
	tasks := make([]*TaskHandle, n)
	for i := range n {
		tasks[i] = seq.Group(fmt.Sprintf("phase %d", i)).Task("step")
	}
	var cost nestedStepCost
	out.mu.Lock()
	for _, st := range out.tasks {
		cost.preds += st.node.PredecessorCount()
	}
	out.mu.Unlock()
	for _, h := range tasks {
		h.Define(func(context.Context) error { return nil })
	}
	if err := seq.Wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	cost.predChecks = out.graph.PredecessorChecks()
	return cost
}

// TestSequenceOfNestedStepsIsLinear guards the Sequence-of-Groups shape:
// each nested step used to carry every step before it, so the Tasks of
// step k held k predecessors (n=4000: 7,998,000 held, 23,994,000 read).
func TestSequenceOfNestedStepsIsLinear(t *testing.T) {
	const n = 2000
	cost := scheduleNestedSteps(t, n)
	t.Logf("nested steps: n=%d preds=%d predChecks=%d", n, cost.preds, cost.predChecks)
	if cost.preds > n {
		t.Errorf("%d nested steps hold %d predecessors (want <= %d)", n, cost.preds, n)
	}
	if cost.predChecks > predChecksPerPredecessor*n {
		t.Errorf("%d nested steps read %d predecessor outcomes (want <= %d)", n, cost.predChecks, predChecksPerPredecessor*n)
	}
}

func BenchmarkScheduleNestedSteps(b *testing.B) {
	for _, n := range []int{1000, 4000, 16000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for range b.N {
				scheduleNestedSteps(b, n)
			}
		})
	}
}

// TestRepeatedStepFailuresStopFollowersOnce guards k failing members of
// one nested step: each failure used to rescan every later step, so the
// cost was k·n.
func TestRepeatedStepFailuresStopFollowersOnce(t *testing.T) {
	const k, n = 50, 200
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard, MaxConcurrency: k})
	defer func() { _ = out.Close() }()
	seq := out.Sequence("phases")
	g := seq.Group("checks")
	boom := errors.New("boom")
	start := make(chan struct{})
	for i := range k {
		g.Task(fmt.Sprintf("check %d", i)).Define(func(context.Context) error { <-start; return boom })
	}
	for i := range n {
		seq.Task(fmt.Sprintf("later %d", i))
	}
	close(start)
	_ = g.Wait()
	checks := out.graph.FollowerChecks()
	t.Logf("k=%d failures, n=%d later steps: follower checks=%d", k, n, checks)
	if checks > n {
		t.Errorf("%d failures examined %d followers of %d later steps (want <= %d)", k, checks, n, n)
	}
}
