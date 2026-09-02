package evo

import "errors"

// Sentinel misuse and lifecycle errors recorded by the output aggregate.
var (
	ErrClosed             = errors.New("evo: output is closed")
	ErrAlreadyResolved    = errors.New("evo: entity is already resolved")
	ErrNoProblems         = errors.New("evo: structured resolution requires problems")
	ErrUnresolvedItem     = errors.New("evo: item has no final state")
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
)
