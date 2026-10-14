package evo

import (
	"github.com/zachbornheimer/evident-output/internal/misuse"
)

// Sentinel misuse and lifecycle errors recorded by the output aggregate.
// Misuse codes live in internal/misuse; graph-owned wait/scope outcomes stay
// in graph. Both are aliased here so the public evo surface does not move.
var (
	errClosed             = misuse.ErrClosed
	errAlreadyResolved    = misuse.ErrAlreadyResolved
	errUnresolvedTask     = misuse.ErrUnresolvedTask
	errInvalidProgress    = misuse.ErrInvalidProgress
	errProgressRegression = misuse.ErrProgressRegression
	errInvalidConfig      = misuse.ErrInvalidConfig
	errRenderer           = misuse.ErrRenderer
	errLimitExceeded      = misuse.ErrLimitExceeded
	errConcurrentRunning  = misuse.ErrConcurrentRunning
	// ErrComputedUnsettled is recorded when Computed.Get is called before
	// the Task that produces the value settled successfully.
	errComputedUnsettled = misuse.ErrComputedUnsettled
	// ErrComputedUnordered is recorded when Computed.Get is called from a
	// Task or container builder that is not ordered after the producing Task.
	ErrComputedUnordered = misuse.ErrComputedUnordered
	// ErrDeclaredInCallback is recorded when a Task, Group, or Sequence is
	// declared from inside a Task's Define callback.
	errDeclaredInCallback = misuse.ErrDeclaredInCallback
	// ErrTerminalWithoutSink is recorded when Config.Terminal is set but
	// Config.Stdout is nil, and the driver cannot report its own destination
	// (it does not implement the Sink() io.Writer accessor). Without either,
	// a non-interactive Finish has nowhere to write the residual/plain
	// projection and would otherwise render nothing at exit 0.
	errTerminalWithoutSink = misuse.ErrTerminalWithoutSink
	// ErrDuplicateSiblingName is recorded when a Task, Group, or Sequence is
	// declared with a name already used by another child of the same parent
	// (§3.1). 1.0 removed get-or-create identity for Task/Group/Sequence
	// declaration: two distinct declarations sharing a name would silently
	// merge into one manifest-reconciliation identity, which the spec
	// forbids and which would make a false "already satisfied" possible.
	// The offending declaration still returns a usable (but orphaned)
	// handle; the real, visible outcome is a Failed task carrying
	// ProblemCodeDuplicateSiblingName (see failDuplicateSiblingLocked).
	errDuplicateSiblingName = misuse.ErrDuplicateSiblingName
)
