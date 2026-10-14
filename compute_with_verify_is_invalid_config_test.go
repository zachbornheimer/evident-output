package evo

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
)

const computedNeverRead = -1

// A Verify that finds the work already satisfied skips the callback, and a
// Compute with no callback run has no value to hand out. Whether the Verify
// finds the work satisfied is runtime state, and misuse must not depend on it,
// so Compute on a Task that has a Verify is misuse at declaration, with the
// same error whatever the Verify would have found. The reader ordered after
// it never starts.
func TestCompute_OnATaskWithAVerifyIsInvalidWhateverTheVerifyFinds(t *testing.T) {
	for name, satisfied := range map[string]bool{"work pending": false, "work already satisfied": true} {
		t.Run(name, func(t *testing.T) {
			out := initRun(Config{stdout: io.Discard, plain: true})
			defer func() { _ = out.close() }()
			made := out.Task("make").Verify(func(context.Context) (bool, error) { return satisfied, nil })
			producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
			var got atomic.Int64
			got.Store(computedNeverRead)
			read := out.Task("read").After(producer).Define(func(context.Context) error {
				got.Store(int64(producer.Get()))
				return nil
			})

			_ = out.finish()

			if !errors.Is(out.err(), errInvalidConfig) {
				t.Errorf("Err() = %v, want ErrInvalidConfig", out.err())
			}
			if got.Load() != computedNeverRead {
				t.Errorf("the reader ran and read %d; it must never run after a producer that was refused", got.Load())
			}
			if err := read.wait(); err == nil {
				t.Errorf("read.Wait() = nil; a reader that never ran must not wait to success")
			}
		})
	}
}
