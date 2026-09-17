package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// ExecSpec declares one managed-state subprocess invocation (spec §8.4).
// Constructing an ExecSpec performs no I/O — Exec performs the operation.
//
// Aliased into internal/engine alongside the rest of the data model.
type ExecSpec = engine.ExecSpec

// Exec declares/reconciles one managed-state subprocess invocation: it
// skips spawning when a prior record proves the operation is already
// current (matching definition, Basis, and every declared Output digest),
// otherwise runs the child and verifies its declared Outputs afterward. A
// nonzero exit, or a declared Output missing after a zero exit, fails the
// operation. ctx must come from a Task's Define callback; called any other
// way it returns ErrNoTaskContext or ErrTaskClosed.
func Exec(ctx context.Context, spec ExecSpec) error { return engine.Exec(ctx, spec) }

// Exec-specific usage and outcome errors (spec §8.4).
var (
	ErrExecSpecMissingExecutable     = engine.ErrExecSpecMissingExecutable
	ErrExecExecutableNotFound        = engine.ErrExecExecutableNotFound
	ErrExecNonzeroExit               = engine.ErrExecNonzeroExit
	ErrExecOutputMissingAfterSuccess = engine.ErrExecOutputMissingAfterSuccess
)
