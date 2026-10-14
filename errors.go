package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
)

// Sentinel misuse and lifecycle errors recorded by the run, aliased from
// internal/misuse so the public evo surface does not move.
var (
	errClosed             = misuse.ErrClosed
	errAlreadyResolved    = misuse.ErrAlreadyResolved
	errUnresolvedTask     = misuse.ErrUnresolvedTask
	errInvalidProgress    = misuse.ErrInvalidProgress
	errProgressRegression = misuse.ErrProgressRegression
	// ErrInvalidConfig: a declaration Evo refuses; Result.Err wraps it.
	ErrInvalidConfig     = misuse.ErrInvalidConfig
	errComputeWithVerify = misuse.ErrComputeWithVerify
	errRenderer          = misuse.ErrRenderer
	errConcurrentRunning = misuse.ErrConcurrentRunning
	// ErrComputedUnsettled: Computed.Get ran before the producing Task settled.
	errComputedUnsettled = misuse.ErrComputedUnsettled
	// ErrComputedUnordered: Computed.Get ran outside an order after the producer.
	ErrComputedUnordered = misuse.ErrComputedUnordered
)
