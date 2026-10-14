package misuse

import (
	"errors"
	"fmt"
)

// Sentinel misuse codes. Engine and graph alias these so the public evo
// surface and errors.Is keep working after the move.
var (
	// ErrClosed is recorded when a call lands after Finish/Close.
	ErrClosed = errors.New("evo: output is closed")
	// ErrAlreadyResolved is recorded when a second terminal verb lands on a
	// Task that already has an outcome.
	ErrAlreadyResolved = errors.New("evo: entity is already resolved")
	// ErrUnresolvedTask is recorded when Finish finds a Task with no final
	// state. Finish renders a per-task hint for it rather than the generic
	// misuse line.
	ErrUnresolvedTask = errors.New("evo: task has no final state")
	// ErrInvalidProgress is recorded when a progress update is negative or
	// completed exceeds total.
	ErrInvalidProgress = errors.New("evo: invalid progress")
	// ErrProgressRegression is recorded when a progress update moves
	// completed backward.
	ErrProgressRegression = errors.New("evo: progress moved backward")
	// ErrDuplicateKey is recorded when two Tasks share a Key.
	ErrDuplicateKey = errors.New("evo: duplicate entity key")
	// ErrInvalidConfig is the one sentinel for a declaration or spec Evo
	// refuses (CONTRACT section 7): After/Verify landing after Define, a Task
	// Defined twice, a nil callback, a duplicate sibling, a declaration from
	// inside a callback, or an invalid Effect, File or Exec spec. Every
	// refusal wraps it, so errors.Is finds it on Result.Err.
	ErrInvalidConfig = errors.New("evo: invalid configuration")
	// ErrComputeWithVerify is ErrInvalidConfig for Compute on a Task that has
	// a Verify: a satisfied Verify skips the callback and leaves no value.
	ErrComputeWithVerify = fmt.Errorf("%w: Compute on a Task with a Verify", ErrInvalidConfig)
	// ErrRenderer is recorded when the configured writer fails.
	ErrRenderer = errors.New("evo: renderer failure")
	// ErrLimitExceeded is recorded when the run declares more entities than
	// Config.MaxEntities allows.
	ErrLimitExceeded = errors.New("evo: resource limit exceeded")
	// ErrReasonSkipOnly is recorded when a Reason built with ForSkip is
	// attached via Kept.
	ErrReasonSkipOnly = errors.New("evo: reason restricted to Skipped was recorded via Kept")
	// ErrReasonWrongTask is recorded when a Reason built with OnTask is
	// attached to a different Task.
	ErrReasonWrongTask = errors.New("evo: reason restricted to another task")
	// ErrConcurrentRunning is recorded when two siblings of a Sequence are
	// Running at once.
	ErrConcurrentRunning = errors.New("evo: two siblings in the same collection are Running simultaneously")
	// ErrComputedUnsettled is why a Computed read was refused: the Task that
	// produces the value has not settled successfully, or settled without
	// producing one.
	ErrComputedUnsettled = errors.New("evo: Computed read before its Task settled")
	// ErrComputedUnordered is why a Computed read was refused: the reader is
	// a Task or container builder that is not ordered after the producing Task.
	ErrComputedUnordered = errors.New("evo: Computed read without an After edge or Sequence order to its Task")
	// ErrDeclaredInCallback is why a declaration was refused: a Task, Group,
	// or Sequence was declared from inside a Task's Define callback.
	ErrDeclaredInCallback = fmt.Errorf("%w: declared from inside a Task callback", ErrInvalidConfig)
	// ErrDryRunDeclaredLate is recorded when DeclareDryRun is called after a
	// durable row has already streamed.
	ErrDryRunDeclaredLate = errors.New("evo: DeclareDryRun called after a durable row was already emitted")
	// ErrTerminalWithoutSink is recorded when Config.Terminal is set but
	// Config.Stdout is nil, and the driver cannot report its own destination.
	ErrTerminalWithoutSink = errors.New("evo: Terminal driver configured without a primary writer")
	// ErrDependencyCycle is what the MisuseSink hears when After edges close
	// a cycle: those Tasks can never start, and each row says so.
	ErrDependencyCycle = errors.New("evo: After dependency cycle")
	// ErrDuplicateSiblingName is recorded when a Task, Group, or Sequence is
	// declared with a name already used by another child of the same parent.
	ErrDuplicateSiblingName = fmt.Errorf("%w: duplicate sibling name", ErrInvalidConfig)
	// ErrKeyAfterDefine is recorded when the task key override is called after
	// Define.
	ErrKeyAfterDefine = errors.New("evo: Key called after Define")
	// ErrBasisAfterDefine is recorded when the task basis is called after
	// Define; the call is ignored.
	ErrBasisAfterDefine = errors.New("evo: Basis called after Define")
)
