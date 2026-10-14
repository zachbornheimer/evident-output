package evo

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
)

// Computed is the value a Task produces for the Tasks declared After it (see
// Compute). The graph decides when it may be read; this records the misuse of
// a refused read.
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
// satisfied skips the callback, so whether a value exists would depend on the
// world instead of the code. Compute on one is ErrInvalidConfig at declaration.
func Compute[T any](task *Task, fn func(context.Context) (T, error)) *Computed[T] {
	c := &Computed[T]{task: task}
	if task == nil || fn == nil {
		if task != nil {
			task.out.recordMisuse(ErrInvalidConfig)
		}
		return c
	}
	o := task.out
	o.mu.Lock()
	st := o.taskStates[task.id]
	o.mu.Unlock()
	if st != nil && len(o.freshness.Tasks().Verifiers(st.node.TaskID())) > 0 {
		o.recordMisuseFor(st.name, fmt.Errorf("%w: %q", errComputeWithVerify, st.name))
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

// producer is the Task that produces the value, for After.
func (c *Computed[T]) producer() *Task {
	if c == nil {
		return nil
	}
	return c.task
}

// Get returns the produced value, valid once the producing Task settled
// successfully by running its callback. Earlier it records
// ErrComputedUnsettled and returns the zero value; a read the declared order
// does not prove (no Sequence order or After edge) records
// ErrComputedUnordered. A refused read inside a Task callback or container
// builder unwinds in every mode, so the Task fails with the sentinel instead
// of settling Done on a zero value. The value is written before the Task
// settles and read only after, so Get is never a data race.
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
