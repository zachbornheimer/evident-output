package evo

import (
	"context"

	iofs "io/fs"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/fs"
)

// File and FileSpec live with the other change aliases until the slice-9 LAYOUT split.

// --- folded from change_aliases.go ---

// The changes a Task's callback performs live in internal/change. Engine
// aliases them so the root package keeps compiling through its own aliases
// until it speaks internal/change directly.

type (
	// FileSet is the desired file states a Patch derived, each bound to the
	// Basis it was derived from.
	fileSet = change.FileSet
	// EffectVerb is the closed set of imperative verbs an opaque Effect may
	// declare. There is deliberately no write verb: file state goes through
	// File (and Patch), never an opaque callback.
	effectVerb = change.EffectVerb
	// Resource names one unit of shared state an operation coordinates on
	// (ZYS-840). It is sealed: only FSResource and LogicalResource make one.
	resource = change.Resource

	// FileFS is the facade evo.File performs every filesystem inspection and
	// mutation through, instead of the os package directly (facade rule): the
	// real filesystem backs every production Output; a test injects a fake to
	// script a filesystem outcome (e.g. a chmod failure) deterministically,
	// without depending on a real disk's permission behavior.
	fileFS = fs.FS
	// osFileFS is FileFS's real implementation, backing every production Output.
	osFileFS = fs.Disk
)

// The complete EffectVerb enum.
const (
	effectAdd       = change.EffectAdd
	effectCreate    = change.EffectCreate
	effectDelete    = change.EffectDelete
	effectInstall   = change.EffectInstall
	effectPush      = change.EffectPush
	effectRemove    = change.EffectRemove
	effectUninstall = change.EffectUninstall
	effectUpdate    = change.EffectUpdate
)

// File declares/reconciles one managed-state file resource; see change.File.
func File(ctx context.Context, spec FileSpec) error { return change.File(ctx, spec.toChange()) }

// Files commits each desired state a Patch derived; see change.Files.
func files(ctx context.Context, files fileSet) error { return change.Files(ctx, files) }

// Patch derives the desired file states a unified text diff describes; see
// change.Patch.
func Patch(ctx context.Context, diff []byte) (fileSet, error) { return change.Patch(ctx, diff) }

// ApplyPatch applies a unified diff directly; see change.ApplyPatch.
func applyPatch(ctx context.Context, diff []byte) error { return change.ApplyPatch(ctx, diff) }

// Exec declares/reconciles one managed-state subprocess invocation; see
// change.Exec.
func Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	r, err := change.Exec(ctx, spec.toChange())
	return ExecResult{Ran: r.Ran, ExitCode: r.ExitCode, Stdout: r.Stdout, Stderr: r.Stderr, Truncated: r.Truncated}, err
}

// Effect performs one opaque mutation inside a Task's Define callback; see
// change.Effect.
func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error {
	return change.Effect(ctx, spec.toChange(), fn)
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
func fSResource(path string) resource { return change.FSResource(path) }

// LogicalResource names non-filesystem shared state (a package manager, a
// remote, a database) where no truthful filesystem path exists. Two logical
// resources overlap only when their names match exactly after surrounding
// whitespace is trimmed; names are not hierarchical.
func logicalResource(name string) resource { return change.LogicalResource(name) }

// File, Exec, Patch, Effect and resource sentinels.
var (
	errFileSpecMissingPath           = change.ErrFileSpecMissingPath
	errFileUnmanagedContentsMissing  = change.ErrFileUnmanagedContentsMissing
	errFilePathIsSymlink             = change.ErrFilePathIsSymlink
	errFilePathTypeMismatch          = change.ErrFilePathTypeMismatch
	errFilePermissionsFailed         = change.ErrFilePermissionsFailed
	errFileConflictingProducer       = change.ErrFileConflictingProducer
	errExecSpecMissingExecutable     = change.ErrExecSpecMissingExecutable
	errExecExecutableNotFound        = change.ErrExecExecutableNotFound
	ErrExecNonzeroExit               = change.ErrExecNonzeroExit
	errExecOutputMissingAfterSuccess = change.ErrExecOutputMissingAfterSuccess
	ErrExecOutputMissing             = change.ErrExecOutputMissingAfterSuccess
	ErrStaleBasis                    = change.ErrStaleBasis
	errPatchMalformed                = change.ErrPatchMalformed
	ErrPatchDoesNotApply             = change.ErrPatchDoesNotApply
	errPatchUnsupported              = change.ErrPatchUnsupported
	errPatchDeleteUnsupported        = change.ErrPatchDeleteUnsupported
	errPatchRenameUnsupported        = change.ErrPatchRenameUnsupported
	errPatchBinaryUnsupported        = change.ErrPatchBinaryUnsupported
	errPatchUnsafePath               = change.ErrPatchUnsafePath
	errPatchStale                    = change.ErrPatchStale
	errEffectVerbInvalid             = change.ErrEffectVerbInvalid
	errEffectObjectMissing           = change.ErrEffectObjectMissing
	errEffectQuantityNotPositive     = change.ErrEffectQuantityNotPositive
	errEffectCallbackMissing         = change.ErrEffectCallbackMissing
	errInvalidPartialEffect          = change.ErrInvalidPartialEffect
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
	errNestedResourceAcquisition = change.ErrNestedResourceAcquisition
	// ErrInvalidResource is returned when a Resource names nothing: an empty
	// path or logical name.
	errInvalidResource = change.ErrInvalidResource
)

type FileSpec struct {
	Path     string
	Contents []byte
	Mode     iofs.FileMode
	Inputs   []string
	Values   map[string]string
	basis    []freshness.Fingerprint
}

func (s FileSpec) toChange() change.FileSpec {
	return change.FileSpec{Path: s.Path, Contents: s.Contents, Mode: s.Mode, Basis: s.basis}
}
