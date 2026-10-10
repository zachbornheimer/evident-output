package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Computed is the value a Task produces for what runs After it.
type Computed[T any] struct{ inner *engine.Computed[T] }

// Compute defines task as the producer of a value and returns it. fn is
// task's Define: its result is what Get returns, and its error fails the
// Task. Name the Computed as a predecessor, and Get it from the Tasks,
// Groups, and Sequences that run After it:
//
//	branches := evo.Compute(repo.Task("prune branches"), prune)
//	repo.Task("report").After(branches).Define(func(ctx context.Context) error {
//	    return report(branches.Get())
//	})
//
// A Computed names its producing Task, so After a Computed is After its
// Task: if the Task failed, was blocked, cancelled, or never started, what
// runs after it never starts.
//
// A Task with a Verify cannot be a producer: a Verify that finds the work
// already satisfied skips fn, so whether a value exists would depend on the
// state of the world and not on the code. Compute on one records
// ErrInvalidConfig at declaration, whatever the Verify would find.
func Compute[T any](task *TaskHandle, fn func(context.Context) (T, error)) *Computed[T] {
	return &Computed[T]{inner: engine.Compute(task.impl(), fn)}
}

// Get returns the produced value. It is valid once the producing Task
// settled successfully by running its callback. Calling it earlier is
// misuse: it records ErrComputedUnsettled with its remedy (a panic under
// Config.Strict) and returns the zero value. Called from a Task callback or
// container builder, a refused read unwinds it instead, so the Task fails with
// the sentinel and does not carry on with a zero value. It is never a data
// race.
func (c *Computed[T]) Get() T {
	if c == nil {
		var zero T
		return zero
	}
	return c.inner.Get()
}

// producer is the Task After waits on when the Computed is a predecessor.
func (c *Computed[T]) producer() *engine.TaskHandle {
	if c == nil {
		return nil
	}
	return c.inner.Producer()
}
