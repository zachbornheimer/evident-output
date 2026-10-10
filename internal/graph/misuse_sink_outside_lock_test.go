package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// lockProbingSink answers whether the graph lock was free each time it heard
// misuse: a strict sink panics, and a panic under the graph lock strands the
// lock, the parked waiters and every ticket behind it.
type lockProbingSink struct {
	g      *Graph
	heard  chan error
	locked chan error
}

func newLockProbingSink() *lockProbingSink {
	return &lockProbingSink{heard: make(chan error, 8), locked: make(chan error, 8)}
}

func (s *lockProbingSink) RecordMisuseFor(_ string, err error) {
	free := make(chan struct{})
	go func() { _ = s.g.Waits(); close(free) }()
	select {
	case <-free:
		s.heard <- err
	case <-time.After(time.Second):
		s.locked <- err
	}
}

func (s *lockProbingSink) expectOutsideLock(t *testing.T, want error) {
	t.Helper()
	select {
	case err := <-s.heard:
		if !errors.Is(err, want) {
			t.Errorf("misuse heard = %v, want %v", err, want)
		}
	case err := <-s.locked:
		t.Fatalf("the misuse sink heard %v with the graph lock held", err)
	case <-time.After(executorTimeout):
		t.Fatalf("the misuse sink never heard %v", want)
	}
}

func TestSlice36_AStallReleaseTellsTheMisuseSinkOutsideTheGraphLock(t *testing.T) {
	sink := newLockProbingSink()
	g := New(record.NewRun(), WithMisuseSink(sink))
	sink.g = g
	waiter, awaited := declare(g, "waiter"), declare(g, "awaited")
	g.AddAfter(awaited, AfterTask(waiter))
	submitWork(g, waiter, settlingWork(g, waiter, func() error { return g.Wait(awaited) }))
	submitWork(g, awaited, settlingWork(g, awaited, func() error { return nil }))

	g.Kick()

	sink.expectOutsideLock(t, ErrWaitDeadlock)
}

func TestSlice36_ACycleBlockTellsTheMisuseSinkOutsideTheGraphLock(t *testing.T) {
	sink := newLockProbingSink()
	g := New(record.NewRun(), WithMisuseSink(sink))
	sink.g = g
	first, second := declare(g, "first"), declare(g, "second")
	g.AddAfter(first, AfterTask(second))
	g.AddAfter(second, AfterTask(first))
	submitWork(g, first, settlingWork(g, first, func() error { return nil }))
	submitWork(g, second, settlingWork(g, second, func() error { return nil }))

	ticket := g.BeginWait(first, 0)
	defer g.EndWait(ticket)
	g.Kick()

	sink.expectOutsideLock(t, ErrDependencyCycle)
}

// A strict sink panics after recording. The wait that found the stall must
// not leave its ticket behind, or a later stall counts a goroutine that is
// gone.
func TestSlice36_AMisuseSinkThatPanicsLeavesNoWaitTicketBehind(t *testing.T) {
	g := New(record.NewRun(), WithMisuseSink(panickingSink{}))
	first, second := declare(g, "first"), declare(g, "second")
	g.AddAfter(first, AfterTask(second))
	g.AddAfter(second, AfterTask(first))
	submitWork(g, first, settlingWork(g, first, func() error { return nil }))
	submitWork(g, second, settlingWork(g, second, func() error { return nil }))

	func() {
		defer func() { _ = recover() }()
		_ = g.Wait(first)
	}()

	if waits := g.Waits(); waits != 0 {
		t.Errorf("parked waits after the strict panic = %d, want 0", waits)
	}
}

type panickingSink struct{}

func (panickingSink) RecordMisuseFor(_ string, err error) { panic(err) }
