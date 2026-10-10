package engine

import (
	"context"
	"fmt"

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

// verifyOn is the name of the Task h refers to when it carries a Verify.
func (h *TaskHandle) verifyOn() (name string, verified bool) {
	o := h.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[h.id]
	if st == nil {
		return "", false
	}
	return st.name, len(o.taskFreshness.Verifiers(st.TaskID())) > 0
}

// Compute defines task as the producer of a value: fn's result becomes
// Computed.Get, and fn's error fails the Task. Compute is task's Define.
//
// A Task with a Verify cannot produce a value: a Verify that finds the work
// already satisfied skips the callback, so whether a value exists would
// depend on the state of the world instead of on the code. Compute on one is
// ErrInvalidConfig at declaration, whatever the Verify would find.
func Compute[T any](task *TaskHandle, fn func(context.Context) (T, error)) *Computed[T] {
	c := &Computed[T]{task: task}
	if task == nil || fn == nil {
		if task != nil {
			task.out.recordMisuse(ErrInvalidConfig)
		}
		return c
	}
	if name, verified := task.verifyOn(); verified {
		task.out.recordMisuseFor(name, fmt.Errorf("%w: Compute on %q, which has a Verify that can skip the callback and leave no value", ErrInvalidConfig, name))
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if c.value == nil {
		o.refuseComputedReadLocked("", ErrComputedUnsettled)
		return zero
	}
	v, err := c.value.Read(o.graph)
	if err == nil {
		return v
	}
	o.refuseComputedReadLocked(c.value.Producer().Name, err)
	return zero
}

// refuseComputedReadLocked records a refused Computed read naming subject. A
// read on the goroutine of a callback or container builder unwinds it with
// err. One from a goroutine the callback started cannot: the graph fails the
// callback's Task when it returns.
func (o *Output) refuseComputedReadLocked(subject string, err error) {
	if subject != "" {
		err = fmt.Errorf("%w: read of %q", err, subject)
	}
	o.recordMisuseFor(subject, err)
	if o.graph.CurrentReader().Unwinds() {
		panic(err)
	}
}
