package engine

import (
	"context"
	"errors"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// Computed is the value a Task produces for the Tasks and containers
// declared After it (see Compute). The graph decides when it may be read
// (graph.Computed); this records the misuse of a read it refused.
type Computed[T any] struct {
	task *TaskHandle
	// value is nil for a producer the Output holds no Task for (a rejected
	// declaration), which can never settle.
	value *graph.Computed[T]
}

// newGraphComputed is the graph's Computed for the Task behind h, or nil
// when the Output holds none.
func newGraphComputed[T any](h *TaskHandle) *graph.Computed[T] {
	o := h.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[h.id]
	if st == nil {
		return nil
	}
	return graph.NewComputed[T](st.node)
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
	c.value = newGraphComputed[T](task)
	task.Define(func(ctx context.Context) error {
		v, err := fn(ctx)
		if err != nil {
			return err
		}
		if c.value != nil {
			c.value.Set(v)
		}
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
// settled successfully by running its callback; earlier, or when a Verify
// found the work already satisfied and the callback never ran, it records
// ErrComputedUnsettled (graph.ErrComputedNoValue in the second case) and
// returns the zero value. A read the declared order does not prove (no
// Sequence order and no After edge to the producer) records
// ErrComputedUnordered and unwinds the calling callback in every mode, so
// the Task or container builder fails with it. The value is written before
// the Task settles and read only after, so Get is never a data race.
func (c *Computed[T]) Get() T {
	var zero T
	if c == nil || c.task == nil || c.task.out == nil {
		return zero
	}
	o := c.task.out
	o.mu.Lock()
	defer o.mu.Unlock()
	if c.value == nil {
		o.recordMisuseFor("", ErrComputedUnsettled)
		return zero
	}
	v, err := c.value.Read(o.graph)
	if err == nil {
		return v
	}
	o.recordMisuseFor(c.value.Producer().Name, err)
	if errors.Is(err, ErrComputedUnordered) {
		// Unordered reads never yield a usable value in any mode: unwind
		// the callback so the Task (or container builder) fails with the
		// sentinel, the same path Strict takes.
		panic(ErrComputedUnordered)
	}
	return zero
}
