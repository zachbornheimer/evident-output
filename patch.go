package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// PatchSpec names a unified diff to derive FileSpecs from. Constructing it
// performs no I/O; Patch performs the operation and never writes.
//
// Aliased into internal/engine alongside the rest of the data model.
type PatchSpec = engine.PatchSpec

// PatchResult is the desired file state Patch derived in memory.
type PatchResult = engine.PatchResult

// Patch identifies the source files a unified diff names, fingerprints
// them as Basis, and derives the desired resulting FileSpecs in memory.
// It mutates nothing on disk. ctx must come from a Task's Define callback.
func Patch(ctx context.Context, spec PatchSpec) (PatchResult, error) {
	return engine.Patch(ctx, spec)
}

// Patch-specific usage errors (callers match with errors.Is).
var (
	ErrPatchDelete    = engine.ErrPatchDelete
	ErrPatchRename    = engine.ErrPatchRename
	ErrPatchBinary    = engine.ErrPatchBinary
	ErrPatchMalformed = engine.ErrPatchMalformed
)
