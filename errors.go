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
	ErrFinishing          = errors.New("evo: output is finishing")
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
)
