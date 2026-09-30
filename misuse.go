package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

var (
	ErrClosed               = engine.ErrClosed
	ErrAlreadyResolved      = engine.ErrAlreadyResolved
	ErrUnresolvedTask       = engine.ErrUnresolvedTask
	ErrDuplicateKey         = engine.ErrDuplicateKey
	ErrInvalidConfig        = engine.ErrInvalidConfig
	ErrRenderer             = engine.ErrRenderer
	ErrLimitExceeded        = engine.ErrLimitExceeded
	ErrConcurrentRunning    = engine.ErrConcurrentRunning
	ErrComputedUnsettled    = engine.ErrComputedUnsettled
	ErrComputedUnordered    = engine.ErrComputedUnordered
	ErrDeclaredInCallback   = engine.ErrDeclaredInCallback
	ErrDryRunDeclaredLate   = engine.ErrDryRunDeclaredLate
	ErrTerminalWithoutSink  = engine.ErrTerminalWithoutSink
	ErrNotStarted           = engine.ErrNotStarted
	ErrWaitDeadlock         = engine.ErrWaitDeadlock
	ErrDuplicateSiblingName = engine.ErrDuplicateSiblingName
	ErrKeyAfterDefine       = engine.ErrKeyAfterDefine
	ErrNoTaskContext        = engine.ErrNoTaskContext
	ErrTaskClosed           = engine.ErrTaskClosed
)
