// This file owns a Task's Define: submitting its callback, and the steps the
// scheduler takes around it, from the freshness checks before it to the
// evidence after it.

package graph

import (
	"context"
	"errors"

	"github.com/zachbornheimer/evident-output/internal/misuse"
	"github.com/zachbornheimer/evident-output/internal/record"
)

const (
	// verifyEvidenceSource names Verify as the source of an evidence phase.
	verifyEvidenceSource = "verify"
	// operationsEvidenceSource names tracked operations (File, Exec) as the
	// after-callback evidence source when the Task registered no Verify
	// (§9.2), so the two never compete for the same phase.
	operationsEvidenceSource = "operations"
	// postconditionUnsatisfied is the summary of a Task whose Verify still
	// fails after its callback succeeded.
	postconditionUnsatisfied = "postcondition not satisfied"
)

// errVerificationUnsatisfied carries a failed post-callback Verify back to
// the scheduler. The Task is already failed with
// record.CodeVerificationUnsatisfied; the error only reads non-nil, so the
// Task's Sequence followers still stop.
var errVerificationUnsatisfied = errors.New("evo: postcondition not satisfied")

// Definition is one Task's Define: its callback, the owner the operations
// inside it act through, and the checks around it.
type Definition[O any] struct {
	Owner    O
	Checks   Checks
	Callback func(context.Context) error
}

// Define submits d as t's work (§7). Define returns at once: the scheduler
// starts the callback once t is eligible, and settles t from what it
// returned. A second Define, or one on a settled Task, is misuse.
func Define[O any](g *Graph, t *Task, d Definition[O]) {
	g.submitDefined(t, WorkOf(g, t, d))
}

// WorkOf is the Work Define submits for d: the run's render work once t is
// claimed, the callback with its checks, and the scheduler's settlement of
// what it returned or how it panicked.
func WorkOf[O any](g *Graph, t *Task, d Definition[O]) Work {
	return Work{
		Started:  func() bool { return g.host.Started(t) },
		Run:      func() error { return runDefinition(g, t, d) },
		Observed: func(err error) { g.observed(t, err) },
		Panicked: func(summary string) { g.fail(t, summary, "") },
	}
}

// submitDefined queues work for t under the run's lock, then starts what may
// start. Misuse is recorded once the lock is free.
func (g *Graph) submitDefined(t *Task, work Work) {
	h := g.host
	h.Lock()
	if h.CheckOpen() != nil {
		h.Unlock()
		return
	}
	switch g.Submit(t, work) {
	case AlreadySettled:
		h.Unlock()
		g.misuse.RecordMisuseFor(t.Name, misuse.ErrAlreadyResolved)
		return
	case AlreadySubmitted:
		h.Unlock()
		g.misuse.RecordMisuseFor("", misuse.ErrInvalidConfig)
		return
	}
	h.Changed(t)
	h.Unlock()
	g.Kick()
}

// runDefinition is t's callback with the steps around it (§7, §9.1, §29,
// §30). A current Basis or a satisfied Verify settles t Satisfied without
// running the callback; a failed observation fails it. After a successful
// callback every Verify must hold, or tracked operations supply the after
// evidence. A callback whose every tracked operation was already current,
// and that committed no Effect, settles Satisfied too.
func runDefinition[O any](g *Graph, t *Task, d Definition[O]) error {
	scope := NewScope(g, t.TaskID(), d.Owner)
	ctx := WithScope(g.Context(), scope)
	if current, observeErr := d.Checks.Current(g.Context()); observeErr != nil {
		g.fail(t, observeErr.Error(), "")
		return callbackOutcome(observeErr)
	} else if current {
		g.satisfy(t)
		return nil
	}
	if evaluated, satisfied, observeErr := d.Checks.Verify(ctx); evaluated {
		g.recordEvidence(t, EvidenceBefore, verifyEvidence(observeErr == nil && satisfied))
		if observeErr != nil {
			g.fail(t, observeErr.Error(), "")
			return callbackOutcome(observeErr)
		}
		if satisfied {
			g.satisfy(t)
			return nil
		}
	}
	g.host.DefinitionStarted(t)
	t.Rec.MarkDefineRan()
	callbackErr := d.Callback(ctx)
	scope.Close()
	g.host.DefinitionFinished(t, callbackErr != nil)
	if callbackErr != nil {
		return callbackOutcome(callbackErr)
	}
	if afterErr := g.checkAfter(ctx, t, d.Checks); afterErr != nil {
		return callbackOutcome(afterErr)
	}
	if _, allCurrent := d.Checks.Operations(); allCurrent && !g.run.HasRecordedEffect(t.ID) {
		g.satisfy(t)
	}
	return nil
}

// checkAfter is the step after a successful callback: every Verify must now
// hold, or, with none, tracked operations supply the after evidence. It
// returns the carrier error when the Task failed.
func (g *Graph) checkAfter(ctx context.Context, t *Task, checks Checks) error {
	if !g.hasPostStateToVerify(t) {
		return nil
	}
	evaluated, satisfied, observeErr := checks.Verify(ctx)
	if !evaluated {
		// Every tracked operation already re-inspected its managed
		// attributes before returning success (§8.2), so having any is
		// proof the state holds (§9.2). Without one, the phase stays
		// unevaluated rather than manufacturing a claim.
		if count, _ := checks.Operations(); count > 0 {
			g.recordEvidence(t, EvidenceAfter, record.EvidencePhase{Evaluated: true, Satisfied: true, Source: operationsEvidenceSource})
		}
		return nil
	}
	g.recordEvidence(t, EvidenceAfter, verifyEvidence(observeErr == nil && satisfied))
	if observeErr != nil {
		g.fail(t, observeErr.Error(), "")
		return callbackOutcome(observeErr)
	}
	if !satisfied {
		g.fail(t, postconditionUnsatisfied, record.CodeVerificationUnsatisfied)
		return errVerificationUnsatisfied
	}
	return nil
}

// hasPostStateToVerify reports whether the state after t's callback is one
// its postcondition can judge. It is not when the callback resolved t
// itself without committing a change (Block, or Kept/Skipped with no Effect
// first: it chose not to converge, E-110, E-112), nor when a dry run or
// preview skipped a mutation the callback planned (the observed state is the
// one before the plan, E-097). A planned run whose callback planned nothing
// is checked as a real run is (E-106).
func (g *Graph) hasPostStateToVerify(t *Task) bool {
	if terminal(t) || (t.HasProposal() && !g.run.HasRecordedEffect(t.ID)) {
		return false
	}
	return !g.host.DryRun() || !g.run.HasPlannedEffect(t.ID)
}

// verifyEvidence is an evaluated Verify phase.
func verifyEvidence(satisfied bool) record.EvidencePhase {
	return record.EvidencePhase{Evaluated: true, Satisfied: satisfied, Source: verifyEvidenceSource}
}

// recordEvidence stores one evidence phase on t and narrates it.
func (g *Graph) recordEvidence(t *Task, phase EvidencePhase, evidence record.EvidencePhase) {
	if phase == EvidenceBefore {
		t.Rec.RecordBeforeEvidence(evidence)
	} else {
		t.Rec.RecordAfterEvidence(evidence)
	}
	g.host.EvidenceEvaluated(t, phase, evidence)
}

// callbackOutcome is a callback's (or an observation's) own error, handed
// back to the scheduler unchanged: the scheduler renders its text verbatim
// as the row's summary, so wrapping it would put plumbing in front of the
// reader. The Task may already be settled; the error only carries the
// outcome back.
func callbackOutcome(outcome error) error { return outcome }
