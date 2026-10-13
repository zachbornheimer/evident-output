package evo

import (
	"context"
	"errors"

	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// This file is the engine's side of the interfaces freshness declares: the
// manifest session's location and the manifest it opens.

// manifestConfig names the manifest this Run reads: the configured
// application id and state directory, scoped to the Run's workspace.
func (o *Output) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: o.changes.Workspace()}
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

// runModeLocked is whether this Run applies its work or only plans it, as
// freshness asks. Callers must already hold o.mu.
func (o *Output) runModeLocked() freshness.RunMode {
	if o.cfg.dryRun {
		return freshness.RunPlans
	}
	return freshness.RunApplies
}
