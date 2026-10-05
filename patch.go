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

// Files commits each desired state in files through File, so dry-run
// planning, already-satisfied, verification, Effects, and the manifest
// behave exactly as they do for File. ctx must come from a Task's Define
// callback.
//
// Each file's source Basis is revalidated while File holds that path,
// immediately before it commits. A file changed since Patch derived it
// fails with ErrStaleBasis and is never overwritten; a file that already
// holds its desired contents is satisfied and records no Effect. Files is
// not a transaction: it stops at the first failing file, and files already
// committed keep their Effects. The caller takes no locks.
func Files(ctx context.Context, files FileSet) error { return engine.Files(ctx, files) }

// ErrStaleBasis is returned by Files when a file changed after Patch
// derived its desired state from it.
var ErrStaleBasis = engine.ErrStaleBasis

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
