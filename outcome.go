package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine"
)

// EntityState is the lifecycle state of an item or task.
//
// C11 naming sweep: these members stay bare (Done, Failed, Blocked, ...)
// rather than gaining a State* prefix to match ConclusionState below —
// prefixing would collide outright with ConclusionState's own StateFailed/
// StateBlocked/StateCancelled/StateWarning constants (same package, same
// identifiers, different types is still a duplicate declaration in Go).
// Renaming ConclusionState's constants instead would ripple into the JSON
// wire (schema 0.3, frozen this release) and every existing golden — this
// is the "document instead" branch the census decision allows.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type EntityState = core.EntityState

// EntityState values — see the type doc comment above for the naming
// rationale (no State* prefix on this block).
const (
	Pending    = core.Pending
	Running    = core.Running
	Done       = core.Done
	Blocked    = core.Blocked
	Failed     = core.Failed
	Skipped    = core.Skipped
	Cancelled  = core.Cancelled
	Empty      = core.Empty
	Incomplete = core.Incomplete
	// NotStarted marks a group task that never ran because an earlier sibling
	// already failed or was cancelled — rendered "-  <name>  not started" and
	// excluded from the conclusion (the group's verdict comes from the
	// failed/cancelled sibling, not from its unstarted followers).
	NotStarted = core.NotStarted
)

// ConclusionState is the human headline for a finished output.
type ConclusionState = core.ConclusionState

// ConclusionState values — the trailing "[state]" band a run can end in.
const (
	StateReady     = core.StateReady
	StateChanged   = core.StateChanged
	StateWarning   = core.StateWarning
	StateBlocked   = core.StateBlocked
	StateFailed    = core.StateFailed
	StateCancelled = core.StateCancelled
	StatePlanned   = core.StatePlanned
)

// Conclusion is the multidimensional meaning of a finished command.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why, and
// EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md §38 for the full layout.
type Conclusion = core.Conclusion

// Default exit codes from architecture §26.
const (
	ExitOK        = core.ExitOK
	ExitBlocked   = core.ExitBlocked
	ExitFailed    = core.ExitFailed
	ExitCancelled = core.ExitCancelled
)

// Resolution names why a Task settled successfully (§29/§30).
type Resolution = core.Resolution

const (
	// ResolutionExecuted marks a Task whose Define callback was entered and
	// returned successfully.
	ResolutionExecuted = core.ResolutionExecuted
	// ResolutionAlreadySatisfied marks a Task whose Define callback was
	// skipped because a pre-Define Verify observed the desired state
	// already held.
	ResolutionAlreadySatisfied = core.ResolutionAlreadySatisfied
	// ResolutionNoWork marks a Task explicitly resolved successfully
	// without ever reaching Define.
	ResolutionNoWork = core.ResolutionNoWork
)

// Block resolves the Task Blocked: a refusal, not a failure. Use it as a
// statement. Inside a Define callback, Block then return nil: Wait still
// reports failure from the Blocked row.
func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	t.impl().Block(summary, options...)
}

func (t *TaskHandle) Cancel(reason string) { t.impl().Cancel(reason) }

// Fail resolves the Task Failed. Use it as a statement. Inside a Define
// callback, return the error as well so Wait sees the callback's own result.
func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	t.impl().Fail(summary, options...)
}

func (t *TaskHandle) Skipped(reason TaxonomyReason) { t.impl().Skipped(reason.inner) }

func (o *Output) Cancel(reason string) { o.impl().Cancel(reason) }

func (o *Output) Fail(summary string, options ...ProblemOption) {
	o.impl().Fail(summary, options...)
}

type TaxonomyReason struct{ inner engine.TaxonomyReason }

func (r TaxonomyReason) Name() string { return r.inner.Name() }

// Reason returns a get-or-create taxonomy Reason by name on the default instance.
func Reason(name string) TaxonomyReason { return TaxonomyReason{inner: engine.Reason(name)} }

// TaxonomyRecord is one accumulated (reason, name) disposition entry —
// recorded by TaskHandle.Skipped, never assembled by hand.
type TaxonomyRecord = core.TaxonomyRecord
