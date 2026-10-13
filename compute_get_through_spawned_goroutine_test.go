package evo

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
)

// A callback that reads a Computed through a goroutine it started is still
// the reader: the goroutine is its hands. Nothing orders it after the
// producer, so the read is refused whether or not the producer happened to
// settle first, and the Task fails with the sentinel. The goroutine cannot
// unwind the callback, so the Task fails when the callback returns.
func TestComputedGet_UnorderedCallbackReadingThroughASpawnedGoroutineIsRefused(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
	read := out.Group("consumers").Task("read")
	var got atomic.Int64
	read.Define(func(context.Context) error {
		_ = made.wait()
		done := make(chan struct{})
		go func() { defer close(done); got.Store(int64(producer.Get())) }()
		<-done
		return nil
	})

	_ = out.finish()

	if err := read.wait(); !errors.Is(err, ErrComputedUnordered) {
		t.Errorf("read.Wait() = %v, want ErrComputedUnordered", err)
	}
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Errorf("Err() = %v, want ErrComputedUnordered", out.err())
	}
	if got.Load() != 0 {
		t.Errorf("Get() through the spawned goroutine = %d, want the zero value of a refused read", got.Load())
	}
}

// The callback's hands reach through any number of goroutines: a read from a
// goroutine started by a goroutine the callback started is refused the same.
func TestComputedGet_UnorderedCallbackReadingThroughAGrandchildGoroutineIsRefused(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
	read := out.Group("consumers").Task("read")
	read.Define(func(context.Context) error {
		_ = made.wait()
		outer := make(chan struct{})
		go func() {
			defer close(outer)
			inner := make(chan struct{})
			go func() { defer close(inner); _ = producer.Get() }()
			<-inner
		}()
		<-outer
		return nil
	})

	_ = out.finish()

	if err := read.wait(); !errors.Is(err, ErrComputedUnordered) {
		t.Errorf("read.Wait() = %v, want ErrComputedUnordered", err)
	}
}

// The same read from a callback ordered after the producer is fine.
func TestComputedGet_OrderedCallbackReadingThroughASpawnedGoroutineGetsTheValue(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	producer := Compute(out.Task("make"), func(context.Context) (int, error) { return 1, nil })
	var got atomic.Int64
	out.Task("read").After(producer).Define(func(context.Context) error {
		done := make(chan struct{})
		go func() { defer close(done); got.Store(int64(producer.Get())) }()
		<-done
		return nil
	})

	_ = out.finish()

	if got.Load() != 1 || out.err() != nil {
		t.Errorf("Get() = %d, Err() = %v; want 1, nil", got.Load(), out.err())
	}
}

// A builder is a reader too: a goroutine it starts is refused the same way,
// and the container fails with the sentinel.
func TestComputedGet_UnorderedBuilderReadingThroughASpawnedGoroutineFailsTheContainer(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	producer := Compute(out.Task("make"), func(context.Context) (int, error) { return 1, nil })
	builder := out.Group("builder")
	builder.define(func(g *Group) {
		done := make(chan struct{})
		go func() { defer close(done); _ = producer.Get() }()
		<-done
		g.Task("child").Define(func(context.Context) error { return nil })
	})

	_ = out.finish()

	if err := builder.wait(); !errors.Is(err, ErrComputedUnordered) {
		t.Errorf("builder.Wait() = %v, want ErrComputedUnordered", err)
	}
}
