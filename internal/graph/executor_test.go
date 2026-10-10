package graph

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// executorTimeout bounds every wait on a goroutine the executor started.
const executorTimeout = 5 * time.Second

// declare adds a Task at the root and returns it, unsubmitted.
func declare(g *Graph, name string) *Task {
	return must(g.AddTask(nil, name, record.TaskInit{State: record.Pending}))
}

// settlingWork is work whose Observed settles the Task from what Run
// returned, the way a caller's resolution step does.
func settlingWork(g *Graph, t *Task, run func() error) Work {
	return Work{
		Run: run,
		Observed: func(err error) {
			if err != nil {
				g.Settle(t, record.Failed)
				return
			}
			g.Settle(t, record.Done)
		},
		Panicked: func(string) { g.Settle(t, record.Failed) },
	}
}

func submitWork(g *Graph, t *Task, work Work) {
	if got := g.Submit(t, work); got != Submitted {
		panic("Submit refused work for " + t.Name)
	}
}

func TestKickRunsEverySubmittedTaskWithinTheConcurrencyCeiling(t *testing.T) {
	g := New(record.NewRun(), WithMaxConcurrency(2))
	var running, peak atomic.Int64
	run := func() error {
		now := running.Add(1)
		for {
			seen := peak.Load()
			if now <= seen || peak.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		running.Add(-1)
		return nil
	}
	tasks := make([]*Task, 6)
	for i := range tasks {
		tasks[i] = declare(g, string(rune('a'+i)))
		submitWork(g, tasks[i], settlingWork(g, tasks[i], run))
	}

	g.Drain()

	for _, task := range tasks {
		if got := task.Rec.State(); got != record.Done {
			t.Errorf("%s = %s, want done", task.Name, got)
		}
	}
	if peak.Load() > 2 || g.MaxObserved() > 2 {
		t.Errorf("ran %d at once (graph saw %d), want at most the ceiling of 2", peak.Load(), g.MaxObserved())
	}
}

func TestObservedHearsWhatRunReturnedAfterItWasRecordedAsWorkErr(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "fails")
	boom := errors.New("boom")
	var recorded error
	work := settlingWork(g, task, func() error { return boom })
	observe := work.Observed
	work.Observed = func(err error) {
		recorded = task.WorkErr()
		observe(err)
	}
	submitWork(g, task, work)

	g.Drain()

	if !errors.Is(recorded, boom) || !errors.Is(task.WorkErr(), boom) {
		t.Errorf("WorkErr was %v when Observed ran and %v after, want %v both times", recorded, task.WorkErr(), boom)
	}
	if got := task.Rec.State(); got != record.Failed {
		t.Errorf("state = %s, want failed", got)
	}
}

func TestStartedRunsOnTheClaimingGoroutineBeforeRun(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "ordered")
	var mu sync.Mutex
	var steps []string
	note := func(step string) {
		mu.Lock()
		defer mu.Unlock()
		steps = append(steps, step)
	}
	work := settlingWork(g, task, func() error { note("run"); return nil })
	work.Started = func() { note("started") }
	submitWork(g, task, work)

	g.Drain()

	if got := strings.Join(steps, ","); got != "started,run" {
		t.Errorf("steps = %s, want started,run", got)
	}
}

func TestPanickedHearsAPanicThatEscapedRun(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "panics")
	var summary string
	work := settlingWork(g, task, func() error { panic("kaput") })
	work.Panicked = func(text string) {
		summary = text
		g.Settle(task, record.Failed)
	}
	submitWork(g, task, work)

	g.Drain()

	if summary != "panic: kaput" {
		t.Errorf("Panicked heard %q, want %q", summary, "panic: kaput")
	}
	if g.Executing() != 0 {
		t.Errorf("%d callbacks still counted as executing after the panic", g.Executing())
	}
}

func TestSubmitReportsWorkThatCannotBeAccepted(t *testing.T) {
	g := New(record.NewRun())
	settled := declare(g, "settled")
	g.Settle(settled, record.Done)
	twice := declare(g, "twice")
	submitWork(g, twice, Work{Run: func() error { return nil }})

	if got := g.Submit(settled, Work{Run: func() error { return nil }}); got != AlreadySettled {
		t.Errorf("Submit on a settled Task = %v, want AlreadySettled", got)
	}
	if got := g.Submit(twice, Work{Run: func() error { return nil }}); got != AlreadySubmitted {
		t.Errorf("second Submit = %v, want AlreadySubmitted", got)
	}
}

func TestInterruptClosesDispatchThenCancelsRowsThenTheContext(t *testing.T) {
	g := New(record.NewRun())
	queued := declare(g, "queued")
	submitWork(g, queued, Work{Run: func() error { return nil }})
	var contextLiveDuringRows bool
	var queuedStateDuringRows record.EntityState

	g.Interrupt("by user", func() {
		contextLiveDuringRows = g.Context().Err() == nil
		queuedStateDuringRows = queued.Rec.State()
		g.Kick()
	})

	if !contextLiveDuringRows {
		t.Error("the run context was cancelled before the rows were cancelled")
	}
	if queuedStateDuringRows != record.Pending {
		t.Errorf("queued Task was %s while the rows were cancelled, want still pending (and never started by Kick)", queuedStateDuringRows)
	}
	if got := queued.Rec.State(); got != record.NotStarted {
		t.Errorf("queued Task = %s after the interrupt, want not_started", got)
	}
	if g.Context().Err() == nil || g.CancelCause() != "by user" {
		t.Errorf("context err %v, cause %q, want cancelled by user", g.Context().Err(), g.CancelCause())
	}
}

func TestWorkSubmittedAfterAnInterruptSettlesNotStarted(t *testing.T) {
	g := New(record.NewRun())
	g.Interrupt("by user", func() {})
	late := declare(g, "late")

	submitWork(g, late, Work{Run: func() error { return nil }})

	if got := late.Rec.State(); got != record.NotStarted {
		t.Errorf("late work = %s, want not_started", got)
	}
}

func TestAWaitOnATaskInADependencyCycleIsAnsweredNotParkedForever(t *testing.T) {
	sink := &misuseLog{}
	g := New(record.NewRun(), WithMisuseSink(sink))
	first, second := declare(g, "first"), declare(g, "second")
	g.AddAfter(first, AfterTask(second))
	g.AddAfter(second, AfterTask(first))
	submitWork(g, first, Work{Run: func() error { return nil }})
	submitWork(g, second, Work{Run: func() error { return nil }})

	ticket := g.BeginWait(first, 0)
	defer g.EndWait(ticket)

	select {
	case <-first.Done():
	case <-time.After(executorTimeout):
		t.Fatal("the stalled wait was never answered")
	}
	if got := first.Rec.State(); got != record.Blocked {
		t.Errorf("first = %s, want blocked by the cycle", got)
	}
	if len(sink.errs) != 1 || !errors.Is(sink.errs[0], ErrDependencyCycle) {
		t.Errorf("misuse heard = %v, want one dependency cycle", sink.errs)
	}
}

func TestBuilderGatePanicFailsTheGateAndRecordsWhy(t *testing.T) {
	g := New(record.NewRun())
	group := g.AddContainer(nil, "group", false)
	gate := g.AddGate(group, func() error { panic("bad builder") })
	g.Submit(gate, Work{})

	g.Drain()

	if got := gate.Rec.State(); got != record.Failed {
		t.Errorf("gate = %s, want failed", got)
	}
	if err := gate.WorkErr(); err == nil || !strings.Contains(err.Error(), "bad builder") {
		t.Errorf("gate WorkErr = %v, want the panic text", err)
	}
}

func TestAbortPendingClosesOneRegisteredChannelOnce(t *testing.T) {
	g := New(record.NewRun())
	abort, release := g.RegisterAbort("gate_1")
	defer release()

	id, ok := g.AbortPending()

	select {
	case <-abort:
	default:
		t.Error("the registered abort channel was not closed")
	}
	if !ok || id != "gate_1" || g.PendingAborts() != 0 {
		t.Errorf("AbortPending = %q, %t with %d still pending, want gate_1 and none left", id, ok, g.PendingAborts())
	}
	if _, again := g.AbortPending(); again {
		t.Error("AbortPending found a gate after the only one was aborted")
	}
}
