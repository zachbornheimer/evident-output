package evo

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// An unordered Get inside a Group builder unwinds the builder, and the
// container's Wait matches the sentinel with errors.Is, like a Task
// callback's does, instead of carrying only its text.
func TestComputedGet_UnorderedBuilderFailsTheContainerWithTheSentinel(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true})
	defer func() { _ = out.close() }()
	producer := Compute(out.Task("make"), func(context.Context) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return 1, nil
	})
	builder := out.Group("builder")
	builder.define(func(g *Group) {
		_ = producer.Get()
		g.Task("child").Define(func(context.Context) error { return nil })
	})

	_ = out.finish()

	if err := builder.wait(); !errors.Is(err, ErrComputedUnordered) {
		t.Errorf("builder.Wait() = %v, want ErrComputedUnordered", err)
	}
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Errorf("Err() = %v, want ErrComputedUnordered", out.err())
	}
}
