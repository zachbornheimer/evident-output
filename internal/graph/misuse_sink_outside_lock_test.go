package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
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
	go func() { _ = s.g.Executing(); close(free) }()
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

func TestSlice36_ACycleBlockTellsTheMisuseSinkOutsideTheGraphLock(t *testing.T) {
	sink := newLockProbingSink()
	g := New(record.NewRun(), WithMisuseSink(sink))
	sink.g = g
	first, second := declare(g, "first"), declare(g, "second")
	g.AddAfter(first, AfterTask(second))
	g.AddAfter(second, AfterTask(first))
	submitWork(g, first, settlingWork(g, first, func() error { return nil }))
	submitWork(g, second, settlingWork(g, second, func() error { return nil }))

	g.BeginDrain()
	g.Kick()

	sink.expectOutsideLock(t, ErrDependencyCycle)
}
