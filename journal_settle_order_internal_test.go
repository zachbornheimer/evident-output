package evo

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
)

// finishSweepEventTypes is the evo.event types a run streams when Finish
// sweeps the Tasks it finds unresolved, in order.
func finishSweepEventTypes(t *testing.T, declare func(*Run)) []string {
	t.Helper()
	var buf bytes.Buffer
	o := initRun(Config{Output: JSONL, Facades: Facades{Stdout: &buf, Stderr: &bytes.Buffer{}}})
	declare(o)
	_ = o.finish()
	return wireEventTypes(decodeWireEvents(t, buf.String()))
}

func TestFinishSweepStreamsSettlesBeforeRunFinished(t *testing.T) {
	types := finishSweepEventTypes(t, func(o *Run) {
		o.Task("forgotten") // never Defined, never resolved: the Finish sweep settles it
		o.Task("ran").Define(func(context.Context) error { return nil })
	})
	t.Logf("types: %v", types)
	if types[len(types)-1] != wire.EventRunFinished {
		t.Errorf("run.finished is not last: %v", types)
	}
	// "forgotten" and "ran" both settle: two task.finished lines, both before run.finished.
	if got := countOf(types, wire.EventTaskFinished); got != 2 {
		t.Errorf("task.finished lines = %d, want 2 (the swept Task and the run one): %v", got, types)
	}
}

func TestAbnormalFinishSweepStreamsNotStartedBeforeRunFinished(t *testing.T) {
	types := finishSweepEventTypes(t, func(o *Run) {
		o.Task("boom").fail("broke")
		o.Task("never") // abnormal finish: the sweep settles it NotStarted
	})
	t.Logf("types: %v", types)
	if types[len(types)-1] != wire.EventRunFinished {
		t.Errorf("run.finished is not last: %v", types)
	}
	if got := countOf(types, wire.EventTaskFinished); got != 2 {
		t.Errorf("task.finished lines = %d, want 2 (the failed Task and the swept one): %v", got, types)
	}
}

func countOf(types []string, want string) int {
	return len(slices.DeleteFunc(slices.Clone(types), func(s string) bool { return s != want }))
}
