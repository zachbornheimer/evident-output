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
// redactor lock this resolution already holds for a pending tail.
func (st *taskState) attachEvidenceTail(state EntityState, problems []Problem) []Problem {
	if (state != Failed && state != Blocked) || st.evidence == nil || st.evidence.Empty() {
		return problems
	}
	for i := range problems {
		if problems[i].Detail == "" && problems[i].EvidenceTail == "" {
			problems[i].Detail = st.evidence.detailText()
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
	// A milestone still owed when the task resolves never gets its claiming
	// Doing — flush it now, before the terminal row, so a Doing-before-
	// Progress loop's last milestone (beginner-8's "always a final n/n") is
	// never silently dropped (E-119 review). Applies to a collection child
	// exactly as it does a standalone task: signalLiveLocked repaints the
	// ledger from current state, so an owed milestone must already be
	// flushed before it runs, not left for a stale next tick that never
	// comes once this task is terminal.
	o.flushOwedMilestoneLocked(st, true)
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
