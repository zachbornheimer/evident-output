package evo_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// A callback that reads a Computed through a goroutine it started is still
// the reader: the goroutine is its hands. Nothing orders it after the
// producer, so the read is refused whether or not the producer happened to
// settle first, and the Task fails with the sentinel. The goroutine cannot
// unwind the callback, so the Task fails when the callback returns.
func TestComputedGet_UnorderedCallbackReadingThroughASpawnedGoroutineIsRefused(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	made := out.Group("producers").Task("make")
	producer := evo.Compute(made, func(context.Context) (int, error) { return 1, nil })
	read := out.Group("consumers").Task("read")
	var got atomic.Int64
	read.Define(func(context.Context) error {
		_ = made.Wait()
		done := make(chan struct{})
		go func() { defer close(done); got.Store(int64(producer.Get())) }()
		<-done
		return nil
	})

	_ = out.Finish()

	if err := read.Wait(); !errors.Is(err, evo.ErrComputedUnordered) {
		t.Errorf("read.Wait() = %v, want ErrComputedUnordered", err)
	}
	if !errors.Is(out.Err(), evo.ErrComputedUnordered) {
		t.Errorf("Err() = %v, want ErrComputedUnordered", out.Err())
	}
	if got.Load() != 0 {
		t.Errorf("Get() through the spawned goroutine = %d, want the zero value of a refused read", got.Load())
	}
}

// The same read from a callback ordered after the producer is fine.
func TestComputedGet_OrderedCallbackReadingThroughASpawnedGoroutineGetsTheValue(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	producer := evo.Compute(out.Task("make"), func(context.Context) (int, error) { return 1, nil })
	var got atomic.Int64
	out.Task("read").After(producer).Define(func(context.Context) error {
		done := make(chan struct{})
		go func() { defer close(done); got.Store(int64(producer.Get())) }()
		<-done
		return nil
	})

	_ = out.Finish()

	if got.Load() != 1 || out.Err() != nil {
		t.Errorf("Get() = %d, Err() = %v; want 1, nil", got.Load(), out.Err())
	}
}

// A builder is a reader too: a goroutine it starts is refused the same way,
// and the container fails with the sentinel.
func TestComputedGet_UnorderedBuilderReadingThroughASpawnedGoroutineFailsTheContainer(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	producer := evo.Compute(out.Task("make"), func(context.Context) (int, error) { return 1, nil })
	builder := out.Group("builder")
	builder.Define(func(g *evo.GroupHandle) {
		done := make(chan struct{})
		go func() { defer close(done); _ = producer.Get() }()
		<-done
		g.Task("child").Define(func(context.Context) error { return nil })
	})

	_ = out.Finish()

	if err := builder.Wait(); !errors.Is(err, evo.ErrComputedUnordered) {
		t.Errorf("builder.Wait() = %v, want ErrComputedUnordered", err)
	}
}
