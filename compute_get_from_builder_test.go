package evo_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// An unordered Get inside a Group builder unwinds the builder, and the
// container's Wait matches the sentinel with errors.Is, like a Task
// callback's does, instead of carrying only its text.
func TestComputedGet_UnorderedBuilderFailsTheContainerWithTheSentinel(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	producer := evo.Compute(out.Task("make"), func(context.Context) (int, error) {
		time.Sleep(10 * time.Millisecond)
		return 1, nil
	})
	builder := out.Group("builder")
	builder.Define(func(g *evo.GroupHandle) {
		_ = producer.Get()
		g.Task("child").Define(func(context.Context) error { return nil })
	})

	_ = out.Finish()

	if err := builder.Wait(); !errors.Is(err, evo.ErrComputedUnordered) {
		t.Errorf("builder.Wait() = %v, want ErrComputedUnordered", err)
	}
	if !errors.Is(out.Err(), evo.ErrComputedUnordered) {
		t.Errorf("Err() = %v, want ErrComputedUnordered", out.Err())
	}
}
