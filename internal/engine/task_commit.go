package engine

import (
	"context"
)

// attachCaptureTail gives a Failed or Blocked row's Problems the capture
// tail the Task already gathered, so the detail a caller collected through
// capture()/PhaseWriter() needs no opt-in (beginner-2). A Problem with its
// own Detail or CaptureTail keeps it; that also avoids re-entering the
// redactor lock this resolution already holds for a pending tail. It fills
// CaptureTail, not Detail, so a summary that already folded the same capture
// text into its own words (e.g. task.Fail("install failed: "+capture.Text()))
// dedupes against it at render time (dedupeCaptureTailAgainstRow) instead of
// repeating it underneath.
func (st *taskState) attachCaptureTail(state EntityState, problems []Problem) []Problem {
	if (state != Failed && state != Blocked) || st.capture == nil || st.capture.Empty() {
		return problems
	}
	for i := range problems {
		if problems[i].Detail == "" && problems[i].CaptureTail == "" {
			problems[i].CaptureTail = st.capture.detailText()
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
	if st.state.Current() != Done {
		return
	}
	runCtx := o.ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	o.commitManifestTaskLocked(runCtx, st.id)
}
