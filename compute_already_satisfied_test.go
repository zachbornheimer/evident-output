package evo_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const computedValueNeverProduced = 42

// A producer whose Verify found the work already satisfied never runs its
// callback, so it has no value to hand out. An ordered reader's Get is
// refused (misuse, zero value) instead of receiving a made-up zero as if the
// producer had answered.
func TestCompute_GetAfterAnAlreadySatisfiedProducerIsRefused(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	made := out.Task("make")
	made.Verify(func(context.Context) (bool, error) { return true, nil })
	producer := evo.Compute(made, func(context.Context) (int, error) { return computedValueNeverProduced, nil })
	var got atomic.Int64
	got.Store(-1)
	out.Task("read").After(producer).Define(func(context.Context) error {
		got.Store(int64(producer.Get()))
		return nil
	})

	_ = out.Finish()

	if !errors.Is(out.Err(), evo.ErrComputedUnsettled) {
		t.Fatalf("Err() = %v, want a refused read matching ErrComputedUnsettled", out.Err())
	}
	if got.Load() != 0 {
		t.Fatalf("Get() = %d, want the zero value of a refused read", got.Load())
	}
}

// A Skip inside the producer's callback still delivers the value the callback
// returned: the callback ran, so there is one.
func TestCompute_GetAfterAProducerSkippedInsideItsCallbackIsTheValue(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	made := out.Task("make")
	producer := evo.Compute(made, func(context.Context) (int, error) {
		made.Skipped(evo.Reason("n/a"))
		return computedValueNeverProduced, nil
	})
	var got atomic.Int64
	out.Task("read").After(producer).Define(func(context.Context) error {
		got.Store(int64(producer.Get()))
		return nil
	})

	_ = out.Finish()

	if got.Load() != computedValueNeverProduced || out.Err() != nil {
		t.Fatalf("Get() = %d, Err() = %v; want %d, nil", got.Load(), out.Err(), computedValueNeverProduced)
	}
}
