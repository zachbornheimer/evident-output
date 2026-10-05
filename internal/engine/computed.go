package engine

import "context"

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
// returns the zero value. The value is written before the Task settles and
// read only after, so Get is never a data race.
func (c *Computed[T]) Get() T {
	var zero T
	if c == nil || c.task == nil || c.task.out == nil {
		return zero
	}
	o := c.task.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[c.task.id]
	if st == nil || stateOutcome(st.state) != predSucceeded {
		name := ""
		if st != nil {
			name = st.name
		}
		o.recordMisuseFor(name, ErrComputedUnsettled)
		return zero
	}
	return c.value
}
