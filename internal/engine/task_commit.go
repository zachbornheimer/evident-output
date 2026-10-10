package engine

// resolveProblems folds extra into st's Problems as it settles into state.
// A Failed or Blocked row's Problems gain the capture tail the Task already
// gathered, so the detail a caller collected through evidence()/PhaseWriter()
// needs no opt-in (beginner-2). The capture is read only when some Problem
// lacks its own detail, which also avoids re-entering the redactor lock this
// resolution already holds for a pending tail.
func (st *taskState) resolveProblems(state EntityState, extra []Problem) {
	tail := ""
	if st.node.Rec.NeedsEvidenceTail(state, extra) && st.evidence != nil && !st.evidence.Empty() {
		tail = st.evidence.detailText()
	}
	st.node.Rec.ResolveProblems(state, extra, tail)
}

// commitSettledLocked writes a settled Task where its readers find it. A
// Group or Sequence child repaints the live ledger; a standalone Task
// commits its row and its own File/Exec ledger rows to scrollback at once,
// so later Printf/Confirm output can never land above finished work. A
// Done Task commits its manifest record; Failed, Blocked, and Cancelled
// never do (spec §8.2/§11.3).
func (o *Output) commitSettledLocked(st *taskState) {
	if st.collection() != nil {
		o.signalLiveLocked(true)
	} else {
		o.commitResolvedTaskLocked(st.id)
		o.commitNamedEffectsLocked(st.id)
	}
	if st.node.Rec.State() != Done {
		return
	}
	o.commitManifestTaskLocked(o.graph.Context(), st.id)
}
