package evo

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
)

const computedValueNeverProduced = 42

// A Skip inside the producer's callback still delivers the value the callback
// returned: the callback ran, so there is one.
func TestCompute_GetAfterAProducerSkippedInsideItsCallbackIsTheValue(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	made := out.Task("make")
	producer := Compute(made, func(context.Context) (int, error) {
		made.skipped(reason("n/a"))
		return computedValueNeverProduced, nil
	})
	var got atomic.Int64
	out.Task("read").After(producer).Define(func(context.Context) error {
		got.Store(int64(producer.Get()))
		return nil
	})

	_ = out.finish()

	if got.Load() != computedValueNeverProduced || out.err() != nil {
		t.Fatalf("Get() = %d, Err() = %v; want %d, nil", got.Load(), out.err(), computedValueNeverProduced)
	}
}
