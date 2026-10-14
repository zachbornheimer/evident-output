package evo

import (
	"context"
	"maps"
	"slices"

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

	// FileFS is the facade evo.File performs every filesystem inspection and
	// mutation through, instead of the os package directly (facade rule): the
	// real filesystem backs every production Output; a test injects a fake to
	// script a filesystem outcome (e.g. a chmod failure) deterministically,
	// without depending on a real disk's permission behavior.
	fileFS = fs.FS
	// osFileFS is FileFS's real implementation, backing every production Output.
	osFileFS = fs.Disk
)

// File declares/reconciles one managed-state file resource; see change.File.
func File(ctx context.Context, spec FileSpec) error { return change.File(ctx, spec.toChange()) }

// Files commits each desired state a Patch derived; see change.Files.
func files(ctx context.Context, files fileSet) error { return change.Files(ctx, files) }

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

// File, Exec, Patch, Effect and resource sentinels.
var (
	ErrExecNonzeroExit   = change.ErrExecNonzeroExit
	ErrExecOutputMissing = change.ErrExecOutputMissingAfterSuccess
	ErrStaleBasis        = change.ErrStaleBasis
	ErrPatchDoesNotApply = change.ErrPatchDoesNotApply
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
	return change.FileSpec{Path: s.Path, Contents: s.Contents, Mode: s.Mode, Basis: specBasis(s.Inputs, s.Values, s.basis)}
}

// specBasis is the Basis a spec declares: its explicit fingerprints, then one
// content fingerprint per Input path and one per Value (in key order), so a
// changed input byte or Value makes the next run re-execute.
func specBasis(inputs []string, values map[string]string, explicit []freshness.Fingerprint) []freshness.Fingerprint {
	basis := slices.Clone(explicit)
	for _, path := range inputs {
		basis = append(basis, freshness.FSPath(path))
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		basis = append(basis, freshness.Value(name, values[name]))
	}
	return basis
}
