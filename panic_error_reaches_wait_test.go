package evo

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

var errPanickedWith = errors.New("panicked with a sentinel")

// A callback that panics with an error is a callback that returned it: the
// waiter matches it with errors.Is instead of parsing the row's text.
func TestWait_CallbackPanickingWithAnErrorReturnsThatError(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	boom := out.Task("boom").Define(func(context.Context) error { panic(errPanickedWith) })

	_ = out.finish()

	if err := boom.wait(); !errors.Is(err, errPanickedWith) {
		t.Errorf("boom.Wait() = %v, want it to match the error the callback panicked with", err)
	}
}

// Under Strict a refused Computed read unwinds the reader. Its Wait matches
// the sentinel and names the producer, so the reader's row says what it read.
func TestWait_StrictRefusedComputedReadNamesTheProducer(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, plain: true, strict: true})
	defer func() { _ = out.close() }()
	producer := Compute(out.Task("make"), func(context.Context) (int, error) { return 1, nil })
	read := out.Task("read").Define(func(context.Context) error { _ = producer.Get(); return nil })

	_ = out.finish()

	err := read.wait()
	if !errors.Is(err, ErrComputedUnordered) {
		t.Fatalf("read.Wait() = %v, want ErrComputedUnordered", err)
	}
	if !strings.Contains(err.Error(), `"make"`) {
		t.Errorf("read.Wait() = %v, want it to name the producer", err)
	}
}
