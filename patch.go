package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// FileSet is the desired file states a Patch derived, each bound to the
// Basis it was derived from. It is opaque: desired contents cannot be
// taken out without their Basis, so the stale-write guard travels with
// them to commit.
type FileSet = engine.FileSet

// Patch derives the desired file states a unified text diff describes and
// mutates nothing. ctx must come from a Task's Define callback; called any
// other way it returns ErrNoTaskContext or ErrTaskClosed.
//
// Each referenced file is read once, and its Basis is the identity of
// exactly the bytes the hunks were applied to. Every hunk must match at
// the line it names. Modifications, mode changes, and file creation are
// supported; deletion, rename/copy, binary, and symlink or submodule forms
// fail with ErrPatchUnsupported (or its specific forms below) rather than
// being approximated. Callers that already know the desired bytes call
// File directly instead of building a diff.
func Patch(ctx context.Context, diff []byte) (FileSet, error) { return engine.Patch(ctx, diff) }

// Patch errors. ErrPatchDeleteUnsupported, ErrPatchRenameUnsupported, and
// ErrPatchBinaryUnsupported each wrap ErrPatchUnsupported.
var (
	ErrPatchMalformed         = engine.ErrPatchMalformed
	ErrPatchDoesNotApply      = engine.ErrPatchDoesNotApply
	ErrPatchUnsupported       = engine.ErrPatchUnsupported
	ErrPatchDeleteUnsupported = engine.ErrPatchDeleteUnsupported
	ErrPatchRenameUnsupported = engine.ErrPatchRenameUnsupported
	ErrPatchBinaryUnsupported = engine.ErrPatchBinaryUnsupported
)
