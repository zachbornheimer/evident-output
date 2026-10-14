package evo

import (
	"context"
	iofs "io/fs"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// File declares/reconciles one managed-state file resource; see change.File.
func File(ctx context.Context, spec FileSpec) error { return change.File(ctx, spec.toChange()) }

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
	return change.FileSpec{Path: s.Path, Contents: s.Contents, Mode: s.Mode, Basis: freshness.SpecBasis(s.Inputs, s.Values, s.basis)}
}
