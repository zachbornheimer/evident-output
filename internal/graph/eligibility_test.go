package graph

import (
	"errors"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// misuseLog is a MisuseSink that keeps what it heard.
type misuseLog struct {
	mu   sync.Mutex
	errs []error
}

func (l *misuseLog) RecordMisuseFor(_ string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, err)
}

// submit declares a Task at the root, submits work for it and places it.
func submit(g *Graph, name string, preds ...Predecessor) *Task {
	t := must(g.AddTask(nil, name, record.TaskInit{State: record.Pending}))
	g.AddAfter(t, preds...)
	g.Enqueue(t, func() error { return nil })
	g.Place(t)
	return t
}

func TestAddTaskRefusesPastTheEntityLimit(t *testing.T) {
	g := New(record.NewRun(), WithMaxEntities(2))
	must(g.AddTask(nil, "a", record.TaskInit{}))
	must(g.AddTask(nil, "b", record.TaskInit{}))

	_, err := g.AddTask(nil, "c", record.TaskInit{})

	if !errors.Is(err, ErrEntityLimit) {
		t.Errorf("third Task past a limit of 2: err = %v, want ErrEntityLimit", err)
	}
}

func TestTerminalTaskIsCountedButNeverRefused(t *testing.T) {
	g := New(record.NewRun(), WithMaxEntities(1))
	must(g.AddTask(nil, "a", record.TaskInit{}))

	task := g.AddTerminalTask("run", record.TaskInit{State: record.Failed})

	if task.Rec.State() != record.Failed {
		t.Errorf("terminal task state = %q, want failed", task.Rec.State())
	}
	if _, err := g.AddTask(nil, "b", record.TaskInit{}); !errors.Is(err, ErrEntityLimit) {
		t.Errorf("the terminal task should count against the limit: err = %v", err)
	}
}

func TestAddTaskRefusesAfterClose(t *testing.T) {
	g := newGraph()
	g.Close()

	if _, err := g.AddTask(nil, "a", record.TaskInit{}); !errors.Is(err, ErrClosed) {
		t.Errorf("AddTask after Close: err = %v, want ErrClosed", err)
	}
}

func TestFailedPredecessorSettlesItsDependentNotStarted(t *testing.T) {
	g := newGraph()
	first := submit(g, "first")
	second := submit(g, "second", AfterTask(first))
	if second.Phase() != PhaseParked || g.Parked() != 1 {
		t.Fatalf("second phase = %v with %d parked, want parked behind first", second.Phase(), g.Parked())
	}

	g.Claim(first)
	g.Settle(first, record.Failed)

	if got := second.Rec.State(); got != record.NotStarted {
		t.Errorf("second state = %q, want not_started", got)
	}
	if got := second.Rec.Summary(); got != NotStartedSummary {
		t.Errorf("second summary = %q, want %q", got, NotStartedSummary)
	}
	select {
	case <-second.Done():
	default:
		t.Error("second.Done() is still open after it settled")
	}
	if g.Parked() != 0 {
		t.Errorf("%d Tasks still parked after the cascade", g.Parked())
	}
}

func TestSucceededPredecessorQueuesItsDependent(t *testing.T) {
	g := newGraph()
	first := submit(g, "first")
	second := submit(g, "second", AfterTask(first))

	g.Claim(first)
	g.Settle(first, record.Done)

	if next := g.NextEligible(); next != second {
		t.Errorf("next eligible = %v, want second", next)
	}
}

func TestBlockCyclesBlocksEveryTaskInTheCycleAndTellsTheSink(t *testing.T) {
	sink := &misuseLog{}
	g := New(record.NewRun(), WithMisuseSink(sink))
	a := must(g.AddTask(nil, "a", record.TaskInit{State: record.Pending}))
	b := must(g.AddTask(nil, "b", record.TaskInit{State: record.Pending}))
	g.AddAfter(a, AfterTask(b))
	g.AddAfter(b, AfterTask(a))
	for _, task := range []*Task{a, b} {
		g.Enqueue(task, func() error { return nil })
		g.Place(task)
	}

	if !g.BlockCycles() {
		t.Fatal("BlockCycles found no cycle")
	}

	for _, task := range []*Task{a, b} {
		if got := task.Rec.State(); got != record.Blocked {
			t.Errorf("%s state = %q, want blocked", task.Name, got)
		}
	}
	if len(sink.errs) != 1 || !errors.Is(sink.errs[0], ErrDependencyCycle) {
		t.Errorf("sink heard %v, want one ErrDependencyCycle", sink.errs)
	}
}

func TestTaskDeclaredAfterAStepFailedSettlesNotStarted(t *testing.T) {
	g := newGraph()
	seq := g.AddContainer(nil, "steps", true)
	first := must(g.AddTask(seq, "first", record.TaskInit{State: record.Pending}))
	g.Enqueue(first, func() error { return nil })
	g.Place(first)
	g.Claim(first)
	g.Settle(first, record.Failed)
	g.FailSequenceFollowers(first)

	late := must(g.AddTask(seq, "late", record.TaskInit{State: record.Pending}))

	if got := late.Rec.State(); got != record.NotStarted {
		t.Errorf("a step declared after the failure = %q, want not_started", got)
	}
}

func TestBuilderGateHoldsTheContainerOpenUntilItConcludes(t *testing.T) {
	g := newGraph()
	group := g.AddContainer(nil, "group", false)
	if gate := g.AddGate(group, func() error { return nil }); gate == nil {
		t.Fatal("AddGate refused an empty container")
	} else {
		g.Enqueue(gate, nil)
		g.Place(gate)
		g.Claim(gate)
		g.Settle(gate, record.Done)
	}

	if group.BuilderFailed() || group.BuilderNotStarted() {
		t.Error("a builder that ran reports failed or not started")
	}
	if again := g.AddGate(group, func() error { return nil }); again != nil {
		t.Error("AddGate accepted a second builder for one container")
	}
}

// reentrantListener calls back into the graph from every Task change.
type reentrantListener struct {
	g     *Graph
	heard int
}

func (l *reentrantListener) TaskChanged(record.TaskID, record.EntityState, record.EntityState) {
	l.heard++
	_ = l.g.Parked()
}

func (l *reentrantListener) EventAppended(record.Event) {}

func TestListenerMayCallTheGraphWhileASettleCascades(t *testing.T) {
	run := record.NewRun()
	g := New(run)
	heard := &reentrantListener{g: g}
	run.SetListener(heard)
	first := submit(g, "first")
	submit(g, "second", AfterTask(first))

	g.Claim(first)
	g.Settle(first, record.Failed)

	if heard.heard < 2 {
		t.Errorf("listener heard %d changes, want at least the failure and the cascade", heard.heard)
	}
}
