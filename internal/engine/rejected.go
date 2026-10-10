package engine

import "fmt"

// A refused declaration (a duplicate sibling name or key, the entity
// limit, a closed Output) still hands the caller a handle, so a chained
// call never panics. The handle keeps why it was refused: Define on it is
// misuse, and Wait says the work never ran instead of reporting success
// for work that did not happen.

// rejectedTask is the handle a refused Task declaration returns. err is
// the refusal and is never nil.
func (o *Output) rejectedTask(err error) *TaskHandle {
	return &TaskHandle{out: o, id: o.graph.NextID("task"), rejected: err}
}

// rejectedGroup is rejectedTask's counterpart for a refused Group or
// Sequence. Everything declared under it is refused for the same reason.
func (o *Output) rejectedGroup(err error) *GroupHandle {
	return &GroupHandle{out: o, id: o.graph.NextID("tasks"), rejected: err}
}

// rejectedWaitOutcome is what Wait returns on a refused declaration: the
// work never started, and why.
func rejectedWaitOutcome(rejected error) error {
	return fmt.Errorf("%w: %w", ErrNotStarted, rejected)
}
