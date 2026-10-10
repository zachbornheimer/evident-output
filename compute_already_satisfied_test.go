package evo_test

import (
	"context"
	"io"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const computedValueNeverProduced = 42

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
