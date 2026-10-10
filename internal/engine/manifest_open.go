package engine

import (
	"context"
	"encoding/hex"

	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// manifestFor returns this Run's manifest Store, opening it on first use
// (spec §11.3) — the same lazy-capture pattern workspace already
// uses for the workspace directory. Every later call, whether it succeeded
// or failed, returns the same cached result: a manifest miss/open failure
// degrades this Run to live-filesystem-only File behavior rather than
// retrying on every call.
//
// Opening waits on the exclusive per-manifest lock, which another run may
// hold for its whole lifetime. That wait happens under manifestOpening,
// never under o.mu: interrupt needs o.mu to cancel the run, so holding it
// here would leave a run queued on the lock deaf to ^C and to its
// caller's deadline until the other run finished.
func (o *Output) manifestFor(ctx context.Context) (*freshness.ManifestStore, error) {
	workspace := o.workspace()
	o.manifestOpening.Lock()
	defer o.manifestOpening.Unlock()
	if store, opened, err := o.openedManifest(); opened {
		return store, err
	}
	cfg := freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: workspace}
	store, err := freshness.OpenManifest(ctx, cfg, freshness.NewSystemManifestEnvironment())
	return o.publishManifest(store, err, manifestApplication(ctx, o.cfg.appID, err))
}

// openedManifest reports the cached result of an earlier manifestFor.
func (o *Output) openedManifest() (*freshness.ManifestStore, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.manifestStore, o.manifestOpened, o.manifestOpenErr
}

// publishManifest caches one open attempt's result for the rest of the
// Run, and the application record every Task commit reuses, and returns
// that result. An open that lands after Close is released at once: Close
// has already run and would never release its lock.
func (o *Output) publishManifest(store *freshness.ManifestStore, err error, app freshness.ApplicationRecord) (*freshness.ManifestStore, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		_ = store.Close()
		return nil, ErrClosed
	}
	o.manifestOpened = true
	o.manifestStore = store
	o.manifestOpenErr = err
	if err == nil {
		o.manifestApp = app
		o.manifestAppDone = true
	}
	return store, err
}

// manifestApplication is this Run's application record (spec §11.2): its
// id plus, when it can be computed, the running binary's freshness. A
// failed open needs none.
func manifestApplication(ctx context.Context, appID string, openErr error) freshness.ApplicationRecord {
	app := freshness.ApplicationRecord{ID: appID}
	if openErr != nil {
		return app
	}
	if appFP, err := freshness.App().Fingerprint(ctx); err == nil {
		app.Fingerprint = "sha256:" + hex.EncodeToString(appFP.Digest[:])
	}
	return app
}
