package evo

import "errors"

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
	ErrDryRunDeclaredLate = errors.New("evo: DeclareDryRun called after a durable row was already emitted")
	// ErrTerminalWithoutSink is recorded when Config.Options supplies a
	// Terminal driver but no primary writer (To), and the driver cannot
	// report its own destination (it does not implement the Sink() io.Writer
	// accessor) — release-gate round 8 finding 2. Without either, a
	// non-interactive Finish has nowhere to write the residual/plain
	// projection and would otherwise render nothing at exit 0.
	ErrTerminalWithoutSink = errors.New("evo: Terminal driver configured without a primary writer")
	// ErrNotStarted is what TaskHandle.Wait returns for a task whose work
	// never ran — a failed or abandoned predecessor, or a run that drained
	// before the task became eligible. Wait once answered such a caller with
	// the zero value of "the error the callback returned", so a waiter
	// rendered a green row over the very next line admitting the work it
	// awaited never started.
	ErrNotStarted = errors.New("evo: awaited task never started")
	// ErrWaitDeadlock is what TaskHandle.Wait returns, and records as
	// misuse, when nothing in the run can ever satisfy the wait: a task
	// waiting on itself, two tasks waiting on each other, or any wait left
	// standing once no callback is running and no task can start. The
	// waiting callback is released with this error so its row states the
	// cycle, rather than the whole run hanging in Finish.
	ErrWaitDeadlock = errors.New("evo: awaited task can never be reached")
)

// errWaitCancelled is what TaskHandle.Wait returns for a task an interrupt
// or an explicit Cancel resolved instead of running. It stays unexported:
// a cancelled run already states itself in the row and the conclusion, and
// the waiter needs "this did not succeed", not a second public name.
var errWaitCancelled = errors.New("evo: awaited task was cancelled")
