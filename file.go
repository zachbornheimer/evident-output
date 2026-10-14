package evo

import (
	"context"
	iofs "io/fs"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// File declares/reconciles one managed-state file resource; see change.File.
func File(ctx context.Context, spec FileSpec) error { return change.File(ctx, spec.toChange()) }

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
