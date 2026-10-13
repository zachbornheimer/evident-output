package engine

import (
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/misuse"
)

// Sentinel misuse and lifecycle errors recorded by the output aggregate.
// Misuse codes live in internal/misuse; graph-owned wait/scope outcomes stay
// in graph. Both are aliased here so the public evo surface does not move.
var (
	ErrClosed             = misuse.ErrClosed
	ErrAlreadyResolved    = misuse.ErrAlreadyResolved
	ErrUnresolvedTask     = misuse.ErrUnresolvedTask
	ErrInvalidProgress    = misuse.ErrInvalidProgress
	ErrProgressRegression = misuse.ErrProgressRegression
	ErrDuplicateKey       = misuse.ErrDuplicateKey
	ErrInvalidConfig      = misuse.ErrInvalidConfig
	ErrRenderer           = misuse.ErrRenderer
	ErrLimitExceeded      = misuse.ErrLimitExceeded
	ErrReasonSkipOnly     = misuse.ErrReasonSkipOnly
	ErrReasonWrongTask    = misuse.ErrReasonWrongTask
	ErrConcurrentRunning  = misuse.ErrConcurrentRunning
	// ErrComputedUnsettled is recorded when Computed.Get is called before
	// the Task that produces the value settled successfully.
	ErrComputedUnsettled = misuse.ErrComputedUnsettled
	// ErrComputedUnordered is recorded when Computed.Get is called from a
	// Task or container builder that is not ordered after the producing Task.
	ErrComputedUnordered = misuse.ErrComputedUnordered
	// ErrDeclaredInCallback is recorded when a Task, Group, or Sequence is
	// declared from inside a Task's Define callback.
	ErrDeclaredInCallback = misuse.ErrDeclaredInCallback
	ErrDryRunDeclaredLate = misuse.ErrDryRunDeclaredLate
	// ErrTerminalWithoutSink is recorded when Config.Terminal is set but
	// Config.Stdout is nil, and the driver cannot report its own destination
	// (it does not implement the Sink() io.Writer accessor). Without either,
	// a non-interactive Finish has nowhere to write the residual/plain
	// projection and would otherwise render nothing at exit 0.
	ErrTerminalWithoutSink = misuse.ErrTerminalWithoutSink
	// ErrNotStarted is what TaskHandle.Wait returns for a task whose work
	// never ran — a failed or abandoned predecessor, a run that drained
	// before the task became eligible, or a task nobody ever Defined. Wait once answered such a caller with
	// the zero value of "the error the callback returned", so a waiter
	// rendered a green row over the very next line admitting the work it
	// awaited never started.
	ErrNotStarted = graph.ErrNotStarted
	// ErrWaitDeadlock is what TaskHandle.Wait returns, and records as
	// misuse, when nothing in the run can ever satisfy the wait: a task
	// waiting on itself, two tasks waiting on each other, or any wait left
	// standing once no callback is running and no task can start. The
	// waiting callback is released with this error so its row states the
	// cycle, rather than the whole run hanging in Finish.
	ErrWaitDeadlock = misuse.ErrWaitDeadlock
	// errDependencyCycle is recorded when After edges close a cycle — a
	// Task after itself, directly, through another Task, or through a Group
	// it belongs to. Every Task in the cycle settles Blocked naming it,
	// instead of Finish waiting forever for work that can never start.
	errDependencyCycle = misuse.ErrDependencyCycle
	// ErrDuplicateSiblingName is recorded when a Task, Group, or Sequence is
	// declared with a name already used by another child of the same parent
	// (§3.1). 1.0 removed get-or-create identity for Task/Group/Sequence
	// declaration: two distinct declarations sharing a name would silently
	// merge into one manifest-reconciliation identity, which the spec
	// forbids and which would make a false "already satisfied" possible.
	// The offending declaration still returns a usable (but orphaned)
	// handle; the real, visible outcome is a Failed task carrying
	// ProblemCodeDuplicateSiblingName (see failDuplicateSiblingLocked).
	ErrDuplicateSiblingName = misuse.ErrDuplicateSiblingName
	// ErrKeyAfterDefine is recorded when TaskHandle.Key is called after
	// Define: §7 freezes dependency/verification/execution configuration at
	// Define, and identity (§3.1) is part of that configuration.
	ErrKeyAfterDefine = misuse.ErrKeyAfterDefine
	// ErrNoTaskContext is what evo.File, evo.Exec and every other operation
	// return for a context that never came from a Define callback (§7.1).
	ErrNoTaskContext = graph.ErrNoTaskContext
	// ErrTaskClosed is what an operation returns for a context captured
	// during a Define callback and reused after that callback returned
	// (§7.1).
	ErrTaskClosed = graph.ErrTaskClosed
	// ErrBasisAfterDefine is recorded when TaskHandle.Basis is called after
	// Define; the call is ignored.
	ErrBasisAfterDefine = misuse.ErrBasisAfterDefine
)
