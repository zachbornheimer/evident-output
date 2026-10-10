package engine

// Wait blocks until every task this Group's children (and their nested
// children, recursively) declared has settled, and returns their aggregate
// outcome. It does not itself serialize eligible
// siblings: every descendant Task was already submitted to the scheduler by
// its own Define call, so Wait only parks on outcomes the scheduler is
// already free to produce concurrently — the same non-serializing guarantee
// TaskHandle.Wait already gives a single Task.
//
// A caller never snapshots/counts failed children to know whether the
// container succeeded: Wait alone is the ordinary control-flow answer, and
// it returns nil only when every descendant actually ran and succeeded.
// Per-child detail remains available through Snapshot.
func (g *GroupHandle) Wait() error {
	if g == nil || g.out == nil {
		return nil
	}
	if g.rejected != nil {
		return rejectedWaitOutcome(g.rejected)
	}
	node := g.out.graph.Container(g.id)
	if node == nil {
		return nil
	}
	outcome := g.out.graph.WaitContainer(node)
	g.out.followRecordAfterWait()
	return outcome
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
