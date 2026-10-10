package engine

import (
	"errors"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// Sentinel misuse and lifecycle errors recorded by the output aggregate.
var (
	ErrClosed             = errors.New("evo: output is closed")
	ErrAlreadyResolved    = errors.New("evo: entity is already resolved")
	ErrUnresolvedTask     = errors.New("evo: task has no final state")
	ErrInvalidProgress    = errors.New("evo: invalid progress")
	ErrProgressRegression = errors.New("evo: progress moved backward")
	ErrDuplicateKey       = errors.New("evo: duplicate entity key")
	ErrInvalidConfig      = errors.New("evo: invalid configuration")
	ErrRenderer           = errors.New("evo: renderer failure")
	ErrLimitExceeded      = errors.New("evo: resource limit exceeded")
	ErrReasonSkipOnly     = errors.New("evo: reason restricted to Skipped was recorded via Kept")
	ErrReasonWrongTask    = errors.New("evo: reason restricted to another task")
	ErrConcurrentRunning  = errors.New("evo: two siblings in the same collection are Running simultaneously")
	// ErrComputedUnsettled is recorded when Computed.Get is called before
	// the Task that produces the value settled successfully.
	ErrComputedUnsettled = graph.ErrComputedUnsettled
	// ErrComputedUnordered is recorded when Computed.Get is called from a
	// Task or container builder that is not ordered after the producing Task.
	ErrComputedUnordered = graph.ErrComputedUnordered
	// ErrDeclaredInCallback is recorded when a Task, Group, or Sequence is
	// declared from inside a Task's Define callback.
	ErrDeclaredInCallback = graph.ErrDeclaredInCallback
	ErrDryRunDeclaredLate = errors.New("evo: DeclareDryRun called after a durable row was already emitted")
	// ErrTerminalWithoutSink is recorded when Config.Terminal is set but
	// Config.Stdout is nil, and the driver cannot report its own destination
	// (it does not implement the Sink() io.Writer accessor). Without either,
	// a non-interactive Finish has nowhere to write the residual/plain
	// projection and would otherwise render nothing at exit 0.
	ErrTerminalWithoutSink = errors.New("evo: Terminal driver configured without a primary writer")
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
	ErrWaitDeadlock = graph.ErrWaitDeadlock
	// errDependencyCycle is recorded when After edges close a cycle — a
	// Task after itself, directly, through another Task, or through a Group
	// it belongs to. Every Task in the cycle settles Blocked naming it,
	// instead of Finish waiting forever for work that can never start.
	errDependencyCycle = graph.ErrDependencyCycle
	// ErrDuplicateSiblingName is recorded when a Task, Group, or Sequence is
	// declared with a name already used by another child of the same parent
	// (§3.1). 1.0 removed get-or-create identity for Task/Group/Sequence
	// declaration: two distinct declarations sharing a name would silently
	// merge into one manifest-reconciliation identity, which the spec
	// forbids and which would make a false "already satisfied" possible.
	// The offending declaration still returns a usable (but orphaned)
	// handle; the real, visible outcome is a Failed task carrying
	// ProblemCodeDuplicateSiblingName (see failDuplicateSiblingLocked).
	ErrDuplicateSiblingName = errors.New("evo: duplicate sibling name")
	// ErrKeyAfterDefine is recorded when TaskHandle.Key is called after
	// Define: §7 freezes dependency/verification/execution configuration at
	// Define, and identity (§3.1) is part of that configuration.
	ErrKeyAfterDefine = errors.New("evo: Key called after Define")
	// ErrNoTaskContext is what taskScope (and, in a later increment,
	// evo.File/evo.Exec) returns for a context that never came from a
	// Define callback (§7.1).
	ErrNoTaskContext = errors.New("evo: context carries no task scope")
	// ErrTaskClosed is what taskScope returns for a context captured during
	// a Define callback and reused after that callback returned (§7.1).
	ErrTaskClosed = errors.New("evo: task scope is closed")
	// ErrBasisAfterDefine is recorded when TaskHandle.Basis is called after
	// Define; the call is ignored.
	ErrBasisAfterDefine = errors.New("evo: Basis called after Define")
)
