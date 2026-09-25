package engine

import (
	"context"
)

// honestOutcome is the one rule between a Task's blocking evidence and its
// terminal state: a Task holding any Problem cannot settle success-class,
// so a Done or Skipped claim over one settles Failed. settleLocked applies
// it to every path that ends a Task.
func (st *taskState) honestOutcome(state EntityState) EntityState {
	if declaresSuccess(state) && len(st.problems) > 0 {
		return Failed
	}
	return state
}

// attachEvidenceTail gives a Failed or Blocked row's Problems the capture
// tail the Task already gathered, so the detail a caller collected through
// evidence()/PhaseWriter() needs no opt-in (beginner-2). A Problem with its
// own Detail or EvidenceTail keeps it; that also avoids re-entering the
// redactor lock this resolution already holds for a pending tail. It fills
// EvidenceTail, not Detail: render's effectiveDetailAndTail already shows
// EvidenceTail alone as the detail body when Detail is empty (identical to
// filling Detail directly), but routing through EvidenceTail is what lets
// render's dedupeEvidenceTailAgainstRow catch the case where a caller's own
// summary already embeds the retained text (P7) — that dedup only ever
// inspects EvidenceTail.
func (st *taskState) attachEvidenceTail(state EntityState, problems []Problem) []Problem {
	if (state != Failed && state != Blocked) || st.evidence == nil || st.evidence.Empty() {
		return problems
	}
	for i := range problems {
		if problems[i].Detail == "" && problems[i].EvidenceTail == "" {
			problems[i].EvidenceTail = st.evidence.detailText()
		}
	}
	return problems
}

// commitSettledLocked writes a settled Task where its readers find it. A
// Group or Sequence child repaints the live ledger; a standalone Task
// commits its row and its own File/Exec ledger rows to scrollback at once,
// so later Printf/Confirm output can never land above finished work. A
// Done Task commits its manifest record; Failed, Blocked, and Cancelled
// never do (spec §8.2/§11.3).
func (o *Output) commitSettledLocked(st *taskState) {
	if st.collection != nil {
		o.signalLiveLocked(true)
	} else {
		o.commitResolvedTaskLocked(st.id)
		o.commitNamedEffectsLocked(st.id)
	}
	if st.state != Done {
		return
	}
	runCtx := o.ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	o.commitManifestTaskLocked(runCtx, st.id)
}
