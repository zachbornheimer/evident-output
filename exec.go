package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Exec is one subprocess invocation. An Exec literal performs no I/O; Run
// spawns it.
type Exec struct {
	// Path is the executable; a bare name resolves through PATH.
	Path string
	// Args is passed to the child literally: no shell, no expansion.
	Args []string
	// Dir is the child's working directory; empty is the Run's workspace.
	Dir string
	// Env is the child's environment in os/exec form ("KEY=VALUE"); nil
	// inherits the parent's.
	Env []string
	// Outputs are the Files and Trees the child produces. Relative paths
	// resolve against Dir. Each must exist after a zero exit, and one with
	// declared Content is verified.
	Outputs Outputs
}

// Outputs is the Files and Trees an Exec produces:
// evo.Outputs{evo.File{...}, evo.Tree{...}}.
type Outputs []fsState

// fsState is the sealed union of File and Tree: an Exec Output.
type fsState interface{ isFSState() }

func (File) isFSState() {}
func (Tree) isFSState() {}

// ExecResult is one Exec attempt's immutable outcome: exit code and the
// captured stdout/stderr tail. Ran is false when Run did not spawn (a
// dry-run plan or a spawn failure).
//
// Stdout and Stderr are the Capture tail Exec retains for the row:
// sanitized, redacted, and bounded (at most 200 completed lines / about
// 256 KiB). Truncated reports that the bound dropped earlier output. When
// you need a tool's complete machine output, have the tool write it to a
// file and declare that file in Outputs.
type ExecResult = engine.ExecResult

// Run spawns the child and waits for it, then verifies Outputs. A nonzero
// exit wraps ErrExecNonzeroExit and a missing Output after a zero exit
// wraps ErrExecOutputMissingAfterSuccess; the result still carries the
// captured attempt. ctx must come from a Task's Define callback.
func (x Exec) Run(ctx context.Context) (ExecResult, error) {
	return ExecResult{}, errNotImplemented
}

// Exec usage and outcome errors.
var (
	// ErrExecPathMissing is an Exec whose Path is empty.
	ErrExecPathMissing               = engine.ErrExecPathMissing
	ErrExecExecutableNotFound        = engine.ErrExecExecutableNotFound
	ErrExecNonzeroExit               = engine.ErrExecNonzeroExit
	ErrExecOutputMissingAfterSuccess = engine.ErrExecOutputMissingAfterSuccess
)

type ProcessRunner = engine.ProcessRunner
type ProcessCommand = engine.ProcessCommand
type ProcessOutcome = engine.ProcessOutcome
