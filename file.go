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
func File(ctx context.Context, spec FileSpec) error { return engine.File(ctx, spec) }

// File-specific usage errors (spec §8.1) plus the resource-admission
// sentinels File, Patch, and Exec share.
var (
	ErrFileSpecMissingPath          = engine.ErrFileSpecMissingPath
	ErrFileUnmanagedContentsMissing = engine.ErrFileUnmanagedContentsMissing
	ErrFilePathIsSymlink            = engine.ErrFilePathIsSymlink
	ErrFilePathTypeMismatch         = engine.ErrFilePathTypeMismatch
	ErrNestedResource               = engine.ErrNestedResource
	ErrStaleBasis                   = engine.ErrStaleBasis
)
