package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
)

// The exported sentinels CONTRACT.md section 7 lists, aliased from the
// internal packages that raise them so errors.Is matches the same value on
// both sides.
var (
	ErrExecNonzeroExit   = change.ErrExecNonzeroExit
	ErrExecOutputMissing = change.ErrExecOutputMissingAfterSuccess
	ErrStaleBasis        = change.ErrStaleBasis
	ErrPatchDoesNotApply = change.ErrPatchDoesNotApply
	// ErrInvalidConfig: a declaration Evo refuses; Result.Err wraps it.
	ErrInvalidConfig = misuse.ErrInvalidConfig
	// ErrComputedUnordered: Computed.Get ran outside an order after the producer.
	ErrComputedUnordered = misuse.ErrComputedUnordered
)

// Sentinel misuse and lifecycle errors recorded by the run, aliased from
// internal/misuse so the public evo surface does not move.
var (
	errClosed             = misuse.ErrClosed
	errAlreadyResolved    = misuse.ErrAlreadyResolved
	errUnresolvedTask     = misuse.ErrUnresolvedTask
	errInvalidProgress    = misuse.ErrInvalidProgress
	errProgressRegression = misuse.ErrProgressRegression
	errComputeWithVerify  = misuse.ErrComputeWithVerify
	errRenderer           = misuse.ErrRenderer
	errConcurrentRunning  = misuse.ErrConcurrentRunning
	// errComputedUnsettled: Computed.Get ran before the producing Task settled.
	errComputedUnsettled = misuse.ErrComputedUnsettled
)
