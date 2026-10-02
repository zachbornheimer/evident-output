package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Patch applies a unified text diff. ctx must come from a Task's Define
// callback; called any other way it returns ErrNoTaskContext or
// ErrTaskClosed.
//
// One diff may touch many files. Every affected path is identified first
// and an unsafe one (absolute, climbing out of the workspace, inside a .git
// directory, or beyond a symlinked directory) fails with ErrPatchUnsafePath.
// Every hunk must match at the line it names, and every file is validated
// against the bytes read before the first commit, so a diff that does not
// apply changes nothing (ErrPatchDoesNotApply). Modify, create (parent
// directories included), delete, rename with or without an edit, and mode
// change are applied; binary, copy, symlink, and submodule forms fail with
// ErrPatchUnsupported.
//
// Each file commits through the same machinery as File: its path is held
// for writing, revalidated against the bytes the diff was validated
// against, and atomically replaced. A file edited concurrently fails with
// ErrPatchStale and is never overwritten. A file that already holds the
// diff's result is satisfied and rewritten by nothing. Under DryRun nothing
// mutates but applicability is still validated.
func Patch(ctx context.Context, diff []byte) error { return engine.ApplyPatch(ctx, diff) }

// Patch errors. ErrPatchBinaryUnsupported wraps ErrPatchUnsupported.
var (
	ErrPatchMalformed         = engine.ErrPatchMalformed
	ErrPatchDoesNotApply      = engine.ErrPatchDoesNotApply
	ErrPatchUnsupported       = engine.ErrPatchUnsupported
	ErrPatchBinaryUnsupported = engine.ErrPatchBinaryUnsupported
	ErrPatchUnsafePath        = engine.ErrPatchUnsafePath
	ErrPatchStale             = engine.ErrPatchStale
)
