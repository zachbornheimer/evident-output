package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// FileSpec declares one managed-state file resource (spec §8). Constructing
// a FileSpec performs no I/O — File performs the operation.
//
// Aliased into internal/engine alongside the rest of the data model.
type FileSpec = engine.FileSpec

// File declares/reconciles one managed-state file resource: it creates a
// missing file, rewrites one whose contents differ from FileSpec.Contents,
// and/or chmods one whose mode differs from FileSpec.Mode — a no-op when
// every managed attribute already matches. ctx must come from a Task's
// Define callback; called any other way it returns ErrNoTaskContext or
// ErrTaskClosed.
//
// File claims its own path for writing with no caller code: overlapping
// File, Basis, and Effect claims in this process wait, and a contended
// wait shows as "waiting for <path>". Across processes, the manifest lock
// serializes Runs that share one manifest namespace. Called while holding
// a resource (inside an Effect callback with a Resource), File returns
// ErrNestedResourceAcquisition.
func File(ctx context.Context, spec FileSpec) error { return engine.File(ctx, spec) }

// File-specific usage errors (spec §8.1).
var (
	ErrFileSpecMissingPath          = engine.ErrFileSpecMissingPath
	ErrFileUnmanagedContentsMissing = engine.ErrFileUnmanagedContentsMissing
	ErrFilePathIsSymlink            = engine.ErrFilePathIsSymlink
	ErrFilePathTypeMismatch         = engine.ErrFilePathTypeMismatch
)

// FileFS is the filesystem facade File operations read and write through
// (Config.FileFS).
type FileFS = engine.FileFS
