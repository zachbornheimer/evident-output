package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/fs"
)

// The changes a Task's callback performs live in internal/change. Engine
// aliases them so the root package keeps compiling through its own aliases
// until it speaks internal/change directly.

type (
	// FileSpec declares one managed-state file resource (spec §8/§8.1).
	FileSpec = change.FileSpec
	// FileSet is the desired file states a Patch derived, each bound to the
	// Basis it was derived from.
	FileSet = change.FileSet
	// ExecSpec declares one managed-state subprocess invocation (spec §8.4).
	ExecSpec = change.ExecSpec
	// ExecResult is one Exec attempt's immutable outcome (spec §8.4).
	ExecResult = change.ExecResult
	// EffectVerb is the closed set of imperative verbs an opaque Effect may
	// declare. There is deliberately no write verb: file state goes through
	// File (and Patch), never an opaque callback.
	EffectVerb = change.EffectVerb
	// EffectSpec describes one aggregate opaque mutation Evo cannot model as
	// desired state. Constructing an EffectSpec performs no I/O.
	EffectSpec = change.EffectSpec
	// Resource names one unit of shared state an operation coordinates on
	// (ZYS-840). It is sealed: only FSResource and LogicalResource make one.
	Resource = change.Resource

	// FileFS is the facade evo.File performs every filesystem inspection and
	// mutation through, instead of the os package directly (facade rule): the
	// real filesystem backs every production Output; a test injects a fake to
	// script a filesystem outcome (e.g. a chmod failure) deterministically,
	// without depending on a real disk's permission behavior.
	FileFS = fs.FS
	// osFileFS is FileFS's real implementation, backing every production Output.
	osFileFS = fs.Disk
)

// The complete EffectVerb enum.
const (
	EffectAdd       = change.EffectAdd
	EffectCreate    = change.EffectCreate
	EffectDelete    = change.EffectDelete
	EffectInstall   = change.EffectInstall
	EffectPush      = change.EffectPush
	EffectRemove    = change.EffectRemove
	EffectUninstall = change.EffectUninstall
	EffectUpdate    = change.EffectUpdate
)

// File declares/reconciles one managed-state file resource; see change.File.
func File(ctx context.Context, spec FileSpec) error { return change.File(ctx, spec) }

// Files commits each desired state a Patch derived; see change.Files.
func Files(ctx context.Context, files FileSet) error { return change.Files(ctx, files) }

// Patch derives the desired file states a unified text diff describes; see
// change.Patch.
func Patch(ctx context.Context, diff []byte) (FileSet, error) { return change.Patch(ctx, diff) }

// ApplyPatch applies a unified diff directly; see change.ApplyPatch.
func ApplyPatch(ctx context.Context, diff []byte) error { return change.ApplyPatch(ctx, diff) }

// Exec declares/reconciles one managed-state subprocess invocation; see
// change.Exec.
func Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) { return change.Exec(ctx, spec) }

// Effect performs one opaque mutation inside a Task's Define callback; see
// change.Effect.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	return change.Effect(ctx, spec, fn)
}

// PartialEffect reports, from an Effect callback, that committed of the
// requested EffectSpec.Quantity really happened before err stopped the rest;
// see change.PartialEffect.
func PartialEffect(committed int, err error) error { return change.PartialEffect(committed, err) }

// FSResource names the filesystem path path. A relative path resolves
// against the Run's workspace directory, the same way FileSpec.Path does,
// and symlinked ancestors resolve to their targets so an alias cannot bypass
// coordination. A claim on a directory overlaps every claim on a path
// beneath it. Construction performs no I/O.
func FSResource(path string) Resource { return change.FSResource(path) }

// LogicalResource names non-filesystem shared state (a package manager, a
// remote, a database) where no truthful filesystem path exists. Two logical
// resources overlap only when their names match exactly after surrounding
// whitespace is trimmed; names are not hierarchical.
func LogicalResource(name string) Resource { return change.LogicalResource(name) }

// File, Exec, Patch, Effect and resource sentinels.
var (
	ErrFileSpecMissingPath           = change.ErrFileSpecMissingPath
	ErrFileUnmanagedContentsMissing  = change.ErrFileUnmanagedContentsMissing
	ErrFilePathIsSymlink             = change.ErrFilePathIsSymlink
	ErrFilePathTypeMismatch          = change.ErrFilePathTypeMismatch
	ErrFilePermissionsFailed         = change.ErrFilePermissionsFailed
	ErrFileConflictingProducer       = change.ErrFileConflictingProducer
	ErrExecSpecMissingExecutable     = change.ErrExecSpecMissingExecutable
	ErrExecExecutableNotFound        = change.ErrExecExecutableNotFound
	ErrExecNonzeroExit               = change.ErrExecNonzeroExit
	ErrExecOutputMissingAfterSuccess = change.ErrExecOutputMissingAfterSuccess
	ErrStaleBasis                    = change.ErrStaleBasis
	ErrPatchMalformed                = change.ErrPatchMalformed
	ErrPatchDoesNotApply             = change.ErrPatchDoesNotApply
	ErrPatchUnsupported              = change.ErrPatchUnsupported
	ErrPatchDeleteUnsupported        = change.ErrPatchDeleteUnsupported
	ErrPatchRenameUnsupported        = change.ErrPatchRenameUnsupported
	ErrPatchBinaryUnsupported        = change.ErrPatchBinaryUnsupported
	ErrPatchUnsafePath               = change.ErrPatchUnsafePath
	ErrPatchStale                    = change.ErrPatchStale
	ErrEffectVerbInvalid             = change.ErrEffectVerbInvalid
	ErrEffectObjectMissing           = change.ErrEffectObjectMissing
	ErrEffectQuantityNotPositive     = change.ErrEffectQuantityNotPositive
	ErrEffectCallbackMissing         = change.ErrEffectCallbackMissing
	ErrInvalidPartialEffect          = change.ErrInvalidPartialEffect
	// ErrNestedResourceAcquisition is returned, without waiting, when code
	// already holding a resource (directly, or through any helper it passed
	// its context to) asks for a second one, or calls Wait on a Task, Group,
	// or Sequence. Holding at most one resource at a time, and never waiting
	// while holding one, is what makes deadlock impossible, so this is
	// misuse even when the second resource is free.
	//
	// Wait takes no context, so it reads the claim from its own goroutine's
	// stack: a goroutine the claim holder starts and then Waits from is not
	// caught, and can deadlock. Pass the held context instead, and never
	// Wait from a goroutine spawned while holding a resource.
	ErrNestedResourceAcquisition = change.ErrNestedResourceAcquisition
	// ErrInvalidResource is returned when a Resource names nothing: an empty
	// path or logical name.
	ErrInvalidResource = change.ErrInvalidResource
)
