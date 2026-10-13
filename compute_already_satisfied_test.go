package evo

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
)

const computedValueNeverProduced = 42

// A Skip inside the producer's callback ends it Excluded, and CONTRACT.md §2
// leaves the Tasks after an Excluded Task NotStarted: nothing reads the value.
func TestCompute_ProducerSkippedInsideItsCallbackLeavesReaderNotStarted(t *testing.T) {
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

	if got.Load() != 0 {
		t.Fatalf("reader ran and read %d after an Excluded producer; want it NotStarted", got.Load())
	}
}
