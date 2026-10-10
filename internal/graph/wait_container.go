package graph

import (
	"cmp"
	"errors"
	"slices"
)

// WaitContainer blocks until every Task c's members (and their nested
// members, recursively) declared has settled, and returns their aggregate
// outcome. It does not serialize eligible siblings: every descendant was
// already submitted by its own Define, so it only parks on outcomes the
// scheduler is already free to produce concurrently, the same non-serializing
// guarantee Wait gives a single Task.
//
// It returns nil only when every descendant actually ran and succeeded.
func (g *Graph) WaitContainer(c *Container) error {
	var stack WaiterStack
	if refusal := g.refuseUnderClaim(c.Name, &stack); refusal != nil {
		return refusal
	}
	builderOutcome := g.awaitBuilders(c, &stack, &InputSeals{})
	// A Wait asks for the container's outcome now, so a Task wired After it
	// may start once the members declared so far succeed.
	g.SealCollection(c)
	return withBuilderOutcome(builderOutcome, g.waitDescendants(g.descendantTasks(c), &stack))
}

// awaitBuilders parks until every topology builder under c has settled, and
// returns why a builder did not declare its children. Nested builders are
// declared by their parent's, so each level is awaited after the one above.
func (g *Graph) awaitBuilders(c *Container, stack *WaiterStack, seen *InputSeals) error {
	var failures []error
	if gate := c.BuilderGate(); gate != nil {
		if outcome := g.waitChecked(gate, stack, seen); outcome != nil {
			failures = append(failures, outcome)
		}
	}
	for _, child := range c.Children() {
		if outcome := g.awaitBuilders(child, stack, seen); outcome != nil {
			failures = append(failures, outcome)
		}
	}
	return errors.Join(failures...)
}

// descendantTasks is every Task declared directly or transitively under c,
// in declaration order: the global ordinal Task, Group and Sequence
// declarations share, so a container whose members interleave Tasks and
// nested containers still joins errors in true declaration order rather than
// "all direct Tasks, then all nested containers".
func (g *Graph) descendantTasks(c *Container) []*Task {
	g.lockRead()
	tasks := appendDescendantTasks(c, nil)
	g.unlockRead()
	slices.SortFunc(tasks, func(a, b *Task) int { return cmp.Compare(a.Declaration, b.Declaration) })
	return tasks
}

// waitDescendants waits on each Task in order and joins the meaningful
// outcomes:
//
//   - nil outcomes contribute nothing;
//   - ErrNotStarted is derivative: a failed, blocked or cancelled
//     predecessor's cascade. It is omitted only when some other descendant
//     contributed a real error, because only then is its cause already
//     represented in this join. When the predecessor sits outside the
//     container the join would otherwise be empty and the wait would report
//     success for work that never ran, so ErrNotStarted surfaces;
//   - every other outcome, including cancellation, stays visible and
//     errors.Is-compatible through errors.Join.
//
// Waiting sequentially does not serialize the descendants' own execution.
func (g *Graph) waitDescendants(tasks []*Task, stack *WaiterStack) error {
	var failures []error
	var notStarted error
	seen := &InputSeals{}
	for _, t := range tasks {
		outcome := g.waitChecked(t, stack, seen)
		switch {
		case outcome == nil:
		case errors.Is(outcome, ErrNotStarted):
			if notStarted == nil {
				notStarted = outcome
			}
		default:
			failures = append(failures, outcome)
		}
	}
	if len(failures) == 0 {
		return notStarted
	}
	return errors.Join(failures...)
}

// withBuilderOutcome folds a container wait's builder outcome into its
// descendants' outcome. A builder that never ran is derivative when some
// descendant already answers, and the whole answer otherwise, so a Group that
// declared nothing because its predecessor failed does not wait to nil.
func withBuilderOutcome(builderOutcome, descendantsOutcome error) error {
	switch {
	case builderOutcome == nil:
		return descendantsOutcome
	case descendantsOutcome == nil:
		return builderOutcome
	case errors.Is(builderOutcome, ErrNotStarted):
		return descendantsOutcome
	default:
		return errors.Join(builderOutcome, descendantsOutcome)
	}
}
