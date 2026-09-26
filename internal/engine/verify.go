package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// verifierFunc is one Verify observation check (§9.1): true means the
// desired state already holds, false means it does not, and an error means
// the observation itself failed.
type verifierFunc func(context.Context) (bool, error)

// verifyEvidenceSource names Verify as the evidence source recorded on a
// TaskSnapshot's Evidence phases (§30).
const verifyEvidenceSource = "verify"

// operationsEvidenceSource names Evo-native tracked operations (evo.File,
// evo.Exec in a later increment) as the after-Define evidence source (§9.2:
// "derive post-definition Evidence when modeled proof exists") — used only
// when the Task registered no explicit Verify, so the two sources never
// compete for the same phase.
const operationsEvidenceSource = "operations"

// errVerificationUnsatisfied is runDefine's internal signal that a
// post-Define Verify observed the desired state was not reached — the task
// is already failed (with ProblemCodeVerificationUnsatisfied) by the time
// this is returned; its only remaining job is to read non-nil to
// executeWork's generic "already resolved by the callback" branch (see
// TaskHandle.finish/resolveScheduled) so sequence followers still cascade.
var errVerificationUnsatisfied = errors.New("evo: postcondition not satisfied")

// Verify registers an advanced current-state observation check (§9.1),
// ANDed with any previously registered check on this Task. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define — a call after Define is ignored and recorded as misuse.
//
// Define's own execution wiring (see runDefine) runs every registered
// Verify twice: once before the callback (with the Task already Running) —
// all true skips the callback entirely and resolves the Task successfully
// with ResolutionAlreadySatisfied; any false enters the callback; an
// observation error fails the Task without entering the callback — and
// once after a successful callback, where any false fails the Task with
// ProblemCodeVerificationUnsatisfied and an observation error fails it
// plainly. Neither check commits a success record on its own; only a fully
// satisfied pass (pre- or post-) does. The after-check is skipped in two
// cases only (hasPostStateToVerify): Define resolved the Task itself
// (Block, or Skipped with no Effect committed first), or a dry run or
// preview skipped an Effect Define planned, so the observed state is the
// one before the plan. A planned run whose Define planned nothing is
// checked like a real one.
func (t *TaskHandle) Verify(fn func(context.Context) (bool, error)) *TaskHandle {
	if t == nil || t.out == nil || fn == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[t.id]
	if st == nil {
		return t
	}
	if st.sched.submitted() {
		o.recordMisuseFor(st.name, ErrInvalidConfig)
		return t
	}
	st.verifiers = append(st.verifiers, verifierFunc(fn))
	return t
}

// Define freezes this Task's dependency/verification configuration and
// submits fn to the scheduler (§7): Define itself returns immediately after
// submission, before fn ever runs; the scheduler starts fn as soon as the
// Task is eligible; fn's error becomes the Task's outcome (see runDefine for
// the Verify-aware resolution wiring); evo.Run/evo.Main wait for every
// submitted Task before returning their Result.
//
// Define returns the same *TaskHandle (ZYS-849 Decisions) so the common
// single-Task shape can be written `return task.Define(fn).Wait()`. This is
// fluent sugar only — it does not change Define's asynchronous submission:
// fn still runs on the scheduler, not inline before Define returns.
//
// A second Define on the same Task is misuse (submitWork's own
// already-submitted guard) — configuration freezes once.
func (t *TaskHandle) Define(fn func(context.Context) error) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	if fn == nil {
		o.recordMisuse(ErrInvalidConfig)
		return t
	}
	o.mu.Lock()
	st := o.taskByRef[t.id]
	if st == nil {
		o.recordMisuse(t.rejected)
		o.mu.Unlock()
		return t
	}
	verifiers := append([]verifierFunc(nil), st.verifiers...)
	o.mu.Unlock()

	t.submitWork(func() error { return t.runDefine(verifiers, fn) })
	return t
}

// runDefine is Define's Verify-aware execution wiring (§7, §9.1, §29/§30).
// It resolves the Task itself on every path except "callback returned an
// error" and "callback returned success with nothing left to check" — those
// two fall through to the scheduler's own generic resolution
// (executeWork/resolveObserved) via passthroughCallbackOutcome, exactly as a
// Verify-less Define always has.
func (t *TaskHandle) runDefine(verifiers []verifierFunc, fn func(context.Context) error) error {
	o := t.out
	scope := &taskScopeHandle{out: o, taskID: t.id}

	if len(verifiers) > 0 {
		allSatisfied, obsErr := evaluateVerifiers(withTaskScope(o.Context(), scope), o, t.id, verifiers)
		if obsErr != nil {
			o.recordEvidencePhase(t.id, evidencePhaseBefore, true, false)
			t.failScheduled(obsErr.Error())
			return passthroughCallbackOutcome(obsErr)
		}
		o.recordEvidencePhase(t.id, evidencePhaseBefore, true, allSatisfied)
		if allSatisfied {
			o.setResolution(t.id, ResolutionAlreadySatisfied)
			t.doneScheduled()
			return nil
		}
	}

	o.mu.Lock()
	o.emitWireEventLocked(wire.EventDefinitionStarted, t.id, nil)
	o.mu.Unlock()
	callbackErr := fn(withTaskScope(o.Context(), scope))
	o.mu.Lock()
	closeTaskScopeLocked(scope)
	o.emitWireEventLocked(wire.EventDefinitionFinished, t.id, map[string]any{"failed": callbackErr != nil})
	o.mu.Unlock()
	if callbackErr != nil {
		return passthroughCallbackOutcome(callbackErr)
	}

	if err := t.checkAfterDefine(verifiers, scope); err != nil {
		return err
	}
	o.setResolution(t.id, ResolutionExecuted)
	return nil
}

// checkAfterDefine is runDefine's post-callback evidence step: every
// registered Verify must now hold, or, with none, tracked operations
// supply the After phase. It returns the carrier error when the Task
// already failed.
func (t *TaskHandle) checkAfterDefine(verifiers []verifierFunc, scope *taskScopeHandle) error {
	o := t.out
	switch {
	case !t.hasPostStateToVerify():
		// The After phase stays unevaluated.
	case len(verifiers) > 0:
		allSatisfied, obsErr := evaluateVerifiers(withTaskScope(o.Context(), scope), o, t.id, verifiers)
		if obsErr != nil {
			o.recordEvidencePhase(t.id, evidencePhaseAfter, true, false)
			t.failScheduled(obsErr.Error())
			return passthroughCallbackOutcome(obsErr)
		}
		o.recordEvidencePhase(t.id, evidencePhaseAfter, true, allSatisfied)
		if !allSatisfied {
			t.failScheduledWithCode(ProblemCodeVerificationUnsatisfied, "postcondition not satisfied")
			return passthroughCallbackOutcome(errVerificationUnsatisfied)
		}
	default:
		// No explicit Verify: derive post-Define Evidence from Evo-native
		// tracked operations when Define recorded any (§9.2). Every
		// operation evo.File appended to manifestOps already re-inspected
		// and confirmed its own managed attributes before returning success
		// (§8.2's "re-inspect / verify every managed attribute" step) — a
		// mismatch there would have made callbackErr non-nil, so reaching
		// here with a non-empty manifestOps set means every tracked
		// operation this Define touched is already known current.
		o.recordOperationsEvidence(t.id)
	}
	return nil
}

// hasPostStateToVerify reports whether the state after this Task's
// Define is one its postcondition can judge. It is not when Define
// resolved the Task itself without committing a change (Kept, Skipped,
// Block: it chose not to converge, so there is no change to verify,
// E-110, E-112), nor when a dry run
// or preview skipped a mutation Define planned (the observed state is
// the state before the plan, E-097). A planned run whose Define planned
// nothing left the real post-state and is checked as a real run is
// (E-106).
func (t *TaskHandle) hasPostStateToVerify() bool {
	o := t.out
	o.mu.Lock()
	st := o.taskByRef[t.id]
	// Block resolves at once, so its terminal Task is never re-checked. A
	// Kept or Skipped inside Define is held as an unratified proposal
	// until the callback's return confirms it; it claims "no change" only
	// while Define committed no Effect (E-112): a Kept after a real
	// mutation still owes its postcondition.
	blocked := st != nil && core.IsTerminalTask(st.state.Current())
	keptUnchanged := st != nil && st.proposed != nil && !o.book.HasRecords(t.id)
	o.mu.Unlock()
	if blocked || keptUnchanged {
		return false
	}
	return !o.cfg.dryRun || !o.hasPlannedEffect(t.id)
}

// recordOperationsEvidence records the after-Define Evidence phase
// from tracked operations (§9.2/§30) when taskID recorded at least one —
// a Task with no Verify and no tracked operation leaves Evidence
// unevaluated rather than manufacturing a claim (§9.2's closing rule).
func (o *Output) recordOperationsEvidence(taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil || len(st.manifestOps) == 0 {
		return
	}
	st.verifyEvidence.After = EvidencePhase{Evaluated: true, Satisfied: true, Source: operationsEvidenceSource}
	o.emitWireEventLocked(wire.EventEvidenceEvaluated, taskID, map[string]any{
		"phase":     "after_definition",
		"evaluated": true,
		"satisfied": true,
		"source":    operationsEvidenceSource,
	})
}

// passthroughCallbackOutcome hands a Define callback's (or a Verify
// observation's) own error back to the scheduler's generic resolution path
// (executeWork/resolveObserved) completely unchanged — that path renders
// err.Error() verbatim as the Task's Fail summary, so wrapping it here would
// prepend internal plumbing text ("runDefine: ...") onto what the reader
// sees, the same reason a caller's own %w-wrapped Fail/Block error keeps
// its wording intact.
// The task itself may already be resolved by the time this runs (a
// pre/post-Verify failure calls failScheduled/failScheduledWithCode before
// returning); this is only ever err's carrier back to executeWork's
// already-terminal branch (see TaskHandle.finish).
func passthroughCallbackOutcome(err error) error {
	return err
}

// evaluateVerifiers runs every verifier in registration order, ANDing their
// results, and stops at the first observation error (§9.1: an observation
// failure is distinct from a false result and takes priority). Each
// verifier's own outcome is emitted as verification.observed (spec §38),
// named by its registration order since Verify registers anonymous funcs.
func evaluateVerifiers(ctx context.Context, o *Output, taskID string, verifiers []verifierFunc) (allSatisfied bool, err error) {
	allSatisfied = true
	for i, v := range verifiers {
		ok, verifyErr := v(ctx)
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventVerificationObserved, taskID, wire.VerificationDoc{
			Name:   fmt.Sprintf("verify_%d", i),
			Status: verificationStatus(ok, verifyErr),
		}.EventPayload())
		o.mu.Unlock()
		if verifyErr != nil {
			return false, verifyErr
		}
		if !ok {
			allSatisfied = false
		}
	}
	return allSatisfied, nil
}

// verificationStatus maps one verifier's outcome to the §36 verification
// status vocabulary (wire.VerificationSatisfied/Unsatisfied/Error).
func verificationStatus(ok bool, err error) string {
	switch {
	case err != nil:
		return wire.VerificationError
	case ok:
		return wire.VerificationSatisfied
	default:
		return wire.VerificationUnsatisfied
	}
}

// evidencePhaseName selects which of a Task's two Verify observation phases
// (§30) a call updates.
type evidencePhaseName int

const (
	evidencePhaseBefore evidencePhaseName = iota
	evidencePhaseAfter
)

// recordEvidencePhase stores one Verify observation phase (§30). satisfied
// is meaningful only when evaluated is true, matching EvidencePhase's own
// documented contract.
func (o *Output) recordEvidencePhase(taskID string, phase evidencePhaseName, evaluated, satisfied bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil {
		return
	}
	recorded := EvidencePhase{Evaluated: evaluated}
	if evaluated {
		recorded.Satisfied = satisfied
		recorded.Source = verifyEvidenceSource
	}
	switch phase {
	case evidencePhaseBefore:
		st.verifyEvidence.Before = recorded
	case evidencePhaseAfter:
		st.verifyEvidence.After = recorded
	}
	o.emitWireEventLocked(wire.EventEvidenceEvaluated, taskID, map[string]any{
		"phase":     evidencePhaseWireName(phase),
		"evaluated": evaluated,
		"satisfied": satisfied,
		"source":    recorded.Source,
	})
}

// evidencePhaseWireName maps an evidencePhaseName to the §38 payload's
// "phase" literal (spec §38 example: "phase": "after_definition").
func evidencePhaseWireName(phase evidencePhaseName) string {
	if phase == evidencePhaseAfter {
		return "after_definition"
	}
	return "before_definition"
}

// setResolution stores why a Task settled successfully (§29/§30).
func (o *Output) setResolution(taskID string, r Resolution) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskByRef[taskID]; st != nil {
		st.resolution = r
	}
}

// failScheduledWithCode is failScheduled with a stable Problem.Code
// attached — the scheduler's own resolution authority (see
// TaskHandle.resolveScheduled), used for outcomes that need a
// machine-matchable code rather than only a rendered summary.
func (t *TaskHandle) failScheduledWithCode(code, summary string) {
	p := core.SanitizeProblem(Problem{Code: code, Summary: txt.Text(summary)})
	t.resolveScheduled(Failed, txt.Text(summary), []Problem{p})
}
