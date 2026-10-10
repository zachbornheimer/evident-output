package engine

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// finishSweepEventTypes is the event types a run that leaves one Task
// forgotten writes to p, in order.
func finishSweepEventTypes(t *testing.T, p Projection) []string {
	t.Helper()
	var buf bytes.Buffer
	o := newOutput("job", to(&buf), withProjection(p))
	o.Task("forgotten") // never Defined, never resolved: the Finish sweep settles it
	o.Task("ran").Define(func(context.Context) error { return nil })
	_ = o.Finish()
	var types []string
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if _, after, ok := strings.Cut(line, `"type":"`); ok {
			rest := after
			types = append(types, rest[:strings.IndexByte(rest, '"')])
		}
	}
	return types
}

func TestFinishSweepJournalsSettlesBeforeOutputFinishedJSONL(t *testing.T) {
	types := finishSweepEventTypes(t, ProjectionJSONL)
	t.Logf("JSONL: %v", types)
	if types[len(types)-1] != "output.finished" {
		t.Errorf("JSONL: output.finished is not last: %v", types)
	}
	if !strings.Contains(strings.Join(types, " "), "task.incomplete") {
		t.Errorf("JSONL: no task.incomplete for the swept task: %v", types)
	}
}

func TestFinishSweepJournalsSettlesBeforeOutputFinishedStream(t *testing.T) {
	types := finishSweepEventTypes(t, ProjectionStreamJSON)
	t.Logf("stream: %v", types)
	if types[len(types)-1] != "output.finished" {
		t.Errorf("stream: output.finished is not last: %v", types)
	}
}

// abnormalSweepEventTypes is the event types a failed run that leaves one
// Task undeclared-to-finish writes to p, in order.
func abnormalSweepEventTypes(t *testing.T, p Projection) []string {
	t.Helper()
	var buf bytes.Buffer
	o := newOutput("job", to(&buf), withProjection(p))
	o.Task("boom").Fail("broke")
	o.Task("never") // abnormal finish: the sweep settles it NotStarted
	_ = o.Finish()
	var types []string
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if _, after, ok := strings.Cut(line, `"type":"`); ok {
			rest := after
			types = append(types, rest[:strings.IndexByte(rest, '"')])
		}
	}
	return types
}

func TestAbnormalFinishSweepJournalsNotStartedBeforeOutputFinished(t *testing.T) {
	for _, p := range []Projection{ProjectionJSONL, ProjectionStreamJSON} {
		types := abnormalSweepEventTypes(t, p)
		t.Logf("%v: %v", p, types)
		if types[len(types)-1] != "output.finished" {
			t.Errorf("%v: output.finished is not last", p)
		}
		if !strings.Contains(strings.Join(types, " "), "task.not_started") {
			t.Errorf("%v: no task.not_started", p)
		}
	}
}
