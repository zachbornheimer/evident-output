package engine

import (
	"context"
	"encoding/hex"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// manifestFor returns this Run's manifest Store, opening it on first use
// (spec §11.3) — the same lazy-capture pattern workspaceDirLocked already
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
func (o *Output) manifestFor(ctx context.Context) (*manifest.Store, error) {
	workspace := o.workspaceDirLocked()
	o.manifestOpening.Lock()
	defer o.manifestOpening.Unlock()
	if store, opened, err := o.openedManifest(); opened {
		return store, err
	}
	cfg := manifest.Config{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: workspace}
	store, err := manifest.Open(ctx, cfg, manifest.NewOSEnvironment())
	o.publishManifest(store, err, manifestApplication(ctx, o.cfg.appID, err))
	return store, err
}

// openedManifest reports the cached result of an earlier manifestFor.
func (o *Output) openedManifest() (*manifest.Store, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.manifestStore, o.manifestOpened, o.manifestOpenErr
}

// publishManifest caches one open attempt's result for the rest of the
// Run, and the application record every Task commit reuses.
func (o *Output) publishManifest(store *manifest.Store, err error, app manifest.ApplicationRecord) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.manifestOpened = true
	o.manifestStore = store
	o.manifestOpenErr = err
	if err == nil {
		o.manifestApp = app
		o.manifestAppDone = true
	}
}

// manifestApplication is this Run's application record (spec §11.2): its
// id plus, when it can be computed, the running binary's fingerprint. A
// failed open needs none.
func manifestApplication(ctx context.Context, appID string, openErr error) manifest.ApplicationRecord {
	app := manifest.ApplicationRecord{ID: appID}
	if openErr != nil {
		return app
	}
	if appFP, err := fingerprint.App().Fingerprint(ctx); err == nil {
		app.Fingerprint = "sha256:" + hex.EncodeToString(appFP.Digest[:])
	}
	return app
}
