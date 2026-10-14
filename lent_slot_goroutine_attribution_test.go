package evo

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// lentSlotOutcome is what each Task's Wait returned and the value the
// goroutine read.
type lentSlotOutcome struct {
	inner, outer error
	read         int
}

// lentSlotRead runs a callback ordered After the producer that starts a
// goroutine reading the producer's value, then Waits on another Task the
// scheduler has not started. At a ceiling of one the callback's slot is lent,
// and that Task runs inline on the callback's goroutine. The goroutine started
// before the inner Task began, so it is the outer callback's hands and its
// read is the outer callback's (ordered, allowed), never the inner Task's
// (unordered).
func lentSlotRead(t *testing.T, ceiling int) lentSlotOutcome {
	t.Helper()
	out := initRun(Config{stdout: io.Discard, plain: true, MaxConcurrency: ceiling})
	defer func() { _ = out.close() }()
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 7, nil })
	inner := out.Group("other").Task("inner")
	outerRunning, innerDefined := make(chan struct{}), make(chan struct{})
	innerRunning, readDone := make(chan struct{}), make(chan struct{})
	var read int
	outer := out.Group("consumers").Task("outer").After(made)
	outer.Define(func(context.Context) error {
		close(outerRunning)
		<-innerDefined
		go func() {
			<-innerRunning
			read = producer.Get()
			close(readDone)
		}()
		return inner.wait()
	})
	<-outerRunning
	inner.Define(func(context.Context) error {
		close(innerRunning)
		<-readDone
		return nil
	})
	close(innerDefined)

	_ = out.finish()
	return lentSlotOutcome{inner: inner.wait(), outer: outer.wait(), read: read}
}

func TestComputedGet_GoroutineStartedBeforeALentSlotRunIsTheLendersReader(t *testing.T) {
	got := lentSlotRead(t, 1)
	if errors.Is(got.inner, ErrComputedUnordered) {
		t.Errorf("inner.Wait() = %v: the outer callback's goroutine's read was charged to the inner Task", got.inner)
	}
	if got.outer != nil || got.read != 7 {
		t.Errorf("outer.Wait() = %v, read = %d; want nil, 7", got.outer, got.read)
	}
}

// The same program at a ceiling of two runs the inner Task on its own slot:
// the outcome must not depend on the ceiling.
func TestComputedGet_GoroutineStartedBeforeARunIsTheLendersReaderAtAnyCeiling(t *testing.T) {
	got := lentSlotRead(t, 2)
	if errors.Is(got.inner, ErrComputedUnordered) || got.outer != nil || got.read != 7 {
		t.Errorf("inner.Wait() = %v, outer.Wait() = %v, read = %d; want no ErrComputedUnordered, nil, 7", got.inner, got.outer, got.read)
	}
}

// The builder analog: a callback starts a goroutine, then Waits on a
// container whose builder has not run (ceiling 1: the builder runs inline on
// the callback's goroutine). The goroutine's Wait on a settled Task is the
// callback's, not the builder's, and must not be refused as inside it.
func TestWait_GoroutineStartedBeforeALentBuilderRunIsNotInsideIt(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, plain: true, MaxConcurrency: 1})
	defer func() { _ = out.close() }()
	made := out.Group("producers").Task("make").Define(func(context.Context) error { return nil })
	if err := made.wait(); err != nil {
		t.Fatal(err)
	}
	built := out.Group("built")
	outerRunning, builtDefined := make(chan struct{}), make(chan struct{})
	builderRunning, waited := make(chan struct{}), make(chan error, 1)
	outer := out.Group("consumers").Task("outer")
	outer.Define(func(context.Context) error {
		close(outerRunning)
		<-builtDefined
		go func() {
			<-builderRunning
			waited <- made.wait()
		}()
		return built.wait()
	})
	<-outerRunning
	built.define(func(g *Group) {
		close(builderRunning)
		err := <-waited
		waited <- err
		g.Task("child").Define(func(context.Context) error { return nil })
	})
	close(builtDefined)

	_ = out.finish()

	if err := <-waited; err != nil && strings.Contains(err.Error(), "inside a container builder") {
		t.Errorf("the outer callback's goroutine Wait(make) = %v: refused as inside a builder it did not start", err)
	}
}
