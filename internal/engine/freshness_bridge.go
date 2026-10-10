package engine

import (
	"context"
	"errors"

	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// This file is the engine's side of the interfaces freshness declares: the
// manifest session's location, the read claims, and the output barrier.

// manifestConfig names the manifest this Run reads: the configured
// application id and state directory, scoped to the Run's workspace.
func (o *Output) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: o.workspace()}
}

// manifestFor returns this Run's manifest Store, opening it on first use
// (spec §11.3). A manifest miss or open failure degrades this Run to
// live-filesystem-only File behavior; see freshness.ManifestSession.Store.
func (o *Output) manifestFor(ctx context.Context) (*freshness.ManifestStore, error) {
	store, err := o.manifest.Store(ctx)
	if errors.Is(err, freshness.ErrManifestSessionClosed) {
		return nil, ErrClosed
	}
	return store, err
}

// readClaims is the Output's resource claims, as freshness asks for them.
type readClaims struct{ out *Output }

// HoldRead runs observe holding a read claim on path's filesystem resource.
func (c readClaims) HoldRead(ctx context.Context, path string, observe func(context.Context) error) error {
	return c.out.holdResource(ctx, FSResource(path), resourceRead, observe)
}

// observeBasis fingerprints basis for one tracked operation (spec §11.1)
// under read claims on each path (ZYS-840).
func (o *Output) observeBasis(ctx context.Context, basis []freshness.Fingerprint) ([]freshness.BasisRecord, error) {
	return freshness.ObserveBasis(ctx, readClaims{out: o}, basis)
}

// outputBarrier is the freshness gates the scheduler keeps for this Run
// (spec §11.6).
func (o *Output) outputBarrier() freshness.OutputBarrier {
	return freshness.NewOutputBarrier(o.graph)
}
