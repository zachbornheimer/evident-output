package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// Computed is the value a Task produces for the Tasks and containers
// declared After it (see Compute).
type Computed[T any] struct {
	task  *TaskHandle
	value T
}

// Compute defines task as the producer of a value: fn's result becomes
// Computed.Get, and fn's error fails the Task. Compute is task's Define.
func Compute[T any](task *TaskHandle, fn func(context.Context) (T, error)) *Computed[T] {
	c := &Computed[T]{task: task}
	if task == nil || fn == nil {
		if task != nil {
			task.out.recordMisuse(ErrInvalidConfig)
		}
		return c
	}
	task.Define(func(ctx context.Context) error {
		v, err := fn(ctx)
		if err != nil {
			return err
		}
		c.value = v
		return nil
	})
	return c
}

// Producer is the Task that produces the value, for After.
func (c *Computed[T]) Producer() *TaskHandle {
	if c == nil {
		return nil
	}
	return c.task
}

// Get returns the produced value. It is valid once the producing Task
// settled successfully; earlier, it records ErrComputedUnsettled and
// returns the zero value. A read the declared order does not prove (no
// Sequence order and no After edge to the producer) records
// ErrComputedUnordered and unwinds the calling callback in every mode, so
// the Task or container builder fails with it. The value is written before the Task settles and
// read only after, so Get is never a data race.
func (c *Computed[T]) Get() T {
	var zero T
	if c == nil || c.task == nil || c.task.out == nil {
		return zero
	}
	o := c.task.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[c.task.id]
	if st == nil || !record.DeclaresSuccess(st.rec.State()) {
		name := ""
		if st != nil {
			name = st.name
		}
		o.recordMisuseFor(name, ErrComputedUnsettled)
		return zero
	}
	if consumer := o.currentConsumerLocked(); consumer != nil && !o.orderedAfterLocked(consumer, st) {
		o.recordMisuseFor(st.name, ErrComputedUnordered)
		// Unordered reads never yield a usable value in any mode: unwind
		// the callback so the Task (or container builder) fails with the
		// sentinel, the same path Strict takes.
		panic(ErrComputedUnordered)
	}
	return c.value
}
