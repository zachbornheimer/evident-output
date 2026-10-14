package evo

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
)

// The same read from a callback ordered after the producer is fine.
func TestComputedGet_OrderedCallbackReadingThroughASpawnedGoroutineGetsTheValue(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, plain: true})
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
