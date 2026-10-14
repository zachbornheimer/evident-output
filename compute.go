package evo

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// Computed is the value a Task produces for the Tasks and containers
// declared After it (see Compute). The graph decides when it may be read
// (graph.Computed); this records the misuse of a read it refused.
type Computed[T any] struct {
	task *Task
	// value is nil for a producer the Output holds no Task for (a rejected
	// declaration), which can never settle.
	value *graph.Computed[T]
}

// Compute defines task as the producer of a value: fn's result becomes
// Computed.Get, and fn's error fails the Task. Compute is task's Define.
//
// A Task with a Verify cannot produce a value: a Verify that finds the work
// already satisfied skips the callback, so whether a value exists would
// depend on the state of the world instead of on the code. Compute on one is
// ErrInvalidConfig at declaration, whatever the Verify would find.
func Compute[T any](task *Task, fn func(context.Context) (T, error)) *Computed[T] {
	c := &Computed[T]{task: task}
	if task == nil || fn == nil {
		if task != nil {
			task.out.recordMisuse(errInvalidConfig)
		}
		return c
	}
	o := task.out
	o.mu.Lock()
	st := o.taskStates[task.id]
	o.mu.Unlock()
	if st != nil && len(o.freshness.Tasks().Verifiers(st.node.TaskID())) > 0 {
		o.recordMisuseFor(st.name, fmt.Errorf("%w: Compute on %q, which has a Verify that can skip the callback and leave no value", errInvalidConfig, st.name))
		return c
	}
	if st != nil {
		c.value = graph.NewComputed[T](st.node)
	}
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
func (c *Computed[T]) producer() *Task {
	if c == nil {
		return nil
	}
	return c.task
}

// Get returns the produced value. It is valid once the producing Task
// settled successfully by running its callback; earlier it records
// ErrComputedUnsettled and returns the zero value. A read the declared order
// does not prove (no Sequence order and no After edge to the producer)
// records ErrComputedUnordered. A refused read from inside a Task callback or
// container builder unwinds it in every mode, so the Task or builder fails
// with the sentinel instead of carrying on with a zero value and settling
// Done. The value is written before the Task settles and read only after, so
// Get is never a data race.
func (c *Computed[T]) Get() T {
	var zero T
	if c == nil || c.task == nil || c.task.out == nil {
		return zero
	}
	o := c.task.out
	// Who is reading is resolved before the Output's lock is taken: finding
	// out may stop the world, and every row's rendering waits on that lock.
	reader := o.graph.CurrentReader()
	o.mu.Lock()
	defer o.mu.Unlock()
	if c.value == nil {
		o.graph.RefuseRead("", errComputedUnsettled, reader)
		return zero
	}
	v, err := c.value.ReadAs(o.graph, reader)
	if err == nil {
		return v
	}
	subject := c.value.Producer().Name
	o.graph.RefuseRead(subject, fmt.Errorf("%w: read of %q", err, subject), reader)
	return zero
}

func (*Computed[T]) predecessor() {}
