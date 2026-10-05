package engine

import (
	"errors"
	"sort"
)

// Wait blocks until every task this Group's children (and their nested
// children, recursively) declared has settled, and returns their aggregate
// outcome (ZYS-849 Decisions). It does not itself serialize eligible
// siblings: every descendant Task was already submitted to the scheduler by
// its own Define call, so Wait only parks on outcomes the scheduler is
// already free to produce concurrently — the same non-serializing guarantee
// TaskHandle.Wait already gives a single Task (see runWaitedWork).
//
// A caller never snapshots/counts failed children to know whether the
// container succeeded: Wait alone is the ordinary control-flow answer.
// Per-child detail remains available through Snapshot.
func (g *GroupHandle) Wait() error {
	if g == nil || g.out == nil {
		return nil
	}
	return waitDescendants(g.out.collectDescendantTasksLocked(g.id))
}

// Wait is Sequence's counterpart to GroupHandle.Wait: the ordered container
// settles once every declared step (and any nested Group/Sequence) has
// settled, honoring the same NotStarted-suppression and error-join rules.
func (s *SequenceHandle) Wait() error {
	if s == nil || s.tasks == nil {
		return nil
	}
	return s.tasks.Wait()
}

// collectDescendantTasksLocked returns every Task declared directly or
// transitively under rootID (a Group/Sequence container id), in declaration
// order — the same global ordinal Task/Group/Sequence declarations share
// (Output.nextDecl), so a container whose children interleave Task and
// nested Group/Sequence declarations still joins errors in true declaration
// order rather than "all direct tasks, then all nested containers".
func (o *Output) collectDescendantTasksLocked(rootID string) []*taskState {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.tasksByRef[rootID]
	if col == nil {
		return nil
	}
	states := make([]*taskState, 0, countDescendantTasksLocked(col))
	states = appendDescendantTasksLocked(col, states)
	sort.Slice(states, func(i, j int) bool { return states[i].declaration < states[j].declaration })
	return states
}

// countDescendantTasksLocked sizes collectDescendantTasksLocked's result
// slice in one pass so the second, appending pass never reallocates. Caller
// must hold o.mu.
func countDescendantTasksLocked(col *tasksState) int {
	n := len(col.tasks)
	for _, child := range col.children {
		n += countDescendantTasksLocked(child)
	}
	return n
}

// appendDescendantTasksLocked recurses through col's own tasks and nested
// children, mirroring collectionFailed/collectionResolved's walk. Caller
// must hold o.mu.
func appendDescendantTasksLocked(col *tasksState, out []*taskState) []*taskState {
	out = append(out, col.tasks...)
	for _, child := range col.children {
		out = appendDescendantTasksLocked(child, out)
	}
	return out
}

// waitDescendants runs TaskHandle.Wait across every descendant in
// declaration order and joins the meaningful outcomes (ZYS-849 Decisions):
//
//   - nil outcomes contribute nothing;
//   - ErrNotStarted is omitted — it is only ever produced by a
//     failed/blocked/cancelled predecessor's cascade (see waitOutcome), and
//     that predecessor's own terminal error is already in this same join, so
//     the predecessor is the cause already represented;
//   - every other outcome, including cancellation, stays visible and
//     errors.Is-compatible through errors.Join.
//
// Waiting sequentially here does not serialize the descendants' own
// execution: each was already submitted to the scheduler before Wait was
// called, so this loop only blocks on outcomes concurrent work is already
// free to produce.
func waitDescendants(states []*taskState) error {
	var errs []error
	for _, st := range states {
		h := st.handle
		if h == nil {
			continue
		}
		err := h.Wait()
		if err == nil {
			continue
		}
		if errors.Is(err, ErrNotStarted) {
			continue
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
