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

// ExecResult is one Exec attempt's immutable outcome: exit code and
// captured stdout/stderr, so a caller can derive structured Problems/Facts
// from a completed subprocess while Evo still owns spawning, capture,
// liveness, cancellation, sanitization, and provenance (spec §8.4/ZYS-850).
// Ran is false when Exec skipped spawning (a current manifest hit or a
// dry-run plan); ordinary callers that don't need the result may ignore it
// with `_, err := evo.Exec(...)`.
//
// Aliased into internal/engine alongside the rest of the data model.
type ExecResult = engine.ExecResult

// Exec declares/reconciles one managed-state subprocess invocation: it
// skips spawning when a prior record proves the operation is already
// current (matching definition, Basis, and every declared Output digest),
// otherwise runs the child and verifies its declared Outputs afterward. A
// nonzero exit, or a declared Output missing after a zero exit, fails the
// operation and wraps ErrExecNonzeroExit / ErrExecOutputMissingAfterSuccess
// respectively — the returned ExecResult still carries the captured
// attempt (Ran=true) in both cases, so a caller intentionally parsing
// nonzero linter output can inspect it via the error path. ctx must come
// from a Task's Define callback; called any other way it returns
// ErrNoTaskContext or ErrTaskClosed.
func Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) { return engine.Exec(ctx, spec) }

// Exec-specific usage and outcome errors (spec §8.4).
var (
	ErrExecSpecMissingExecutable     = engine.ErrExecSpecMissingExecutable
	ErrExecExecutableNotFound        = engine.ErrExecExecutableNotFound
	ErrExecNonzeroExit               = engine.ErrExecNonzeroExit
	ErrExecOutputMissingAfterSuccess = engine.ErrExecOutputMissingAfterSuccess
)
