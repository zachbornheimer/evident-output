package evo

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// waiterOutcome is what each Task's wait returned and the value the goroutine
// read.
type waiterOutcome struct {
	inner, outer error
	read         int
}

// waiterRead runs a callback ordered After the producer that starts a
// goroutine reading the producer's value, then waits on another Task the
// scheduler has not started. The goroutine started before the inner Task
// began, so it is the outer callback's hands and its read is the outer
// callback's (ordered, allowed), never the inner Task's (unordered).
func waiterRead(t *testing.T, ceiling int) waiterOutcome {
	t.Helper()
	out := initRun(Config{MaxConcurrency: ceiling, Output: Plain, Facades: Facades{Stderr: io.Discard}})
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
	return waiterOutcome{inner: inner.wait(), outer: outer.wait(), read: read}
}

// The inner Task runs on a worker of its own, concurrently with the outer
// callback that waits on it, and the goroutine the outer callback started
// still reads as the outer callback's.
func TestComputedGet_GoroutineStartedBeforeAWaitedTaskBeganReadsAsTheWaitersAtCeiling2(t *testing.T) {
	got := waiterRead(t, 2)
	if errors.Is(got.inner, ErrComputedUnordered) || got.outer != nil || got.read != 7 {
		t.Errorf("inner.Wait() = %v, outer.Wait() = %v, read = %d; want no ErrComputedUnordered, nil, 7", got.inner, got.outer, got.read)
	}
}

// The builder analog: a callback starts a goroutine, then waits on a
// container whose builder has not run. The goroutine's wait on a settled Task
// is the callback's, not the builder's, and must not be refused as inside it.
func TestWait_GoroutineStartedBeforeAWaitedBuilderRanIsNotInsideIt(t *testing.T) {
	out := initRun(Config{MaxConcurrency: 2, Output: Plain, Facades: Facades{Stderr: io.Discard}})
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
