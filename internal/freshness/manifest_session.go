// This file owns a Run's manifest session: opening its ManifestStore lazily,
// once, and saving and releasing it when the Run ends.

package freshness

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
)

// ErrManifestSessionClosed is returned when the manifest is asked for after
// the session was closed.
var ErrManifestSessionClosed = errors.New("freshness: manifest session closed")

// appFingerprintPrefix names the digest algorithm of an application record's
// fingerprint.
const appFingerprintPrefix = "sha256:"

// ManifestSession is one Run's lazily opened ManifestStore (spec §11.3),
// plus the application record every Task commit reuses.
//
// A purely opaque consumer's Run never asks for the store, so it never
// creates a manifest file at all.
type ManifestSession struct {
	// where names the manifest this Run reads, asked for once at first open.
	where func() ManifestConfig
	// opening serializes the first open without holding mu across the
	// blocking lock wait: another Run may hold the manifest lock for its
	// whole lifetime, and Close needs mu to cancel this one.
	opening sync.Mutex

	mu         sync.Mutex
	store      *ManifestStore
	opened     bool
	openErr    error
	app        ApplicationRecord
	closed     bool
	finishErr  error
	missWarned bool
	unsaved    bool
}

// NewManifestSession returns a session over the manifest where names.
func NewManifestSession(where func() ManifestConfig) *ManifestSession {
	return &ManifestSession{where: where}
}

// Store returns this Run's manifest Store, opening it on first use. Every
// later call, whether the open succeeded or failed, returns the same cached
// result: a manifest miss or open failure degrades this Run to
// live-filesystem-only File behavior rather than retrying on every call.
//
// Opening waits on the exclusive per-manifest lock, which another run may
// hold for its whole lifetime. That wait happens under opening, never under
// mu, so a Run queued on the lock stays deaf neither to ^C nor to its
// caller's deadline.
func (s *ManifestSession) Store(ctx context.Context) (*ManifestStore, error) {
	cfg := s.where()
	s.opening.Lock()
	defer s.opening.Unlock()
	if store, opened, openErr := s.cached(); opened {
		return store, openErr
	}
	store, openErr := OpenManifest(ctx, cfg, NewSystemManifestEnvironment())
	return s.publish(store, openErr, applicationRecord(ctx, cfg.AppID, openErr))
}

// cached reports the result of an earlier Store call.
func (s *ManifestSession) cached() (store *ManifestStore, opened bool, openErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store, s.opened, s.openErr
}

// publish caches one open attempt's result for the rest of the Run. An open
// that lands after Close is released at once: Close has already run and
// would never release its lock.
func (s *ManifestSession) publish(store *ManifestStore, openErr error, app ApplicationRecord) (*ManifestStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = store.Close()
		return nil, ErrManifestSessionClosed
	}
	s.opened, s.store, s.openErr = true, store, openErr
	if openErr == nil {
		s.app = app
	}
	return store, openErr
}

// applicationRecord is this Run's application record (spec §11.2): its id
// plus, when it can be computed, the running binary's fingerprint. A failed
// open needs none.
func applicationRecord(ctx context.Context, appID string, openErr error) ApplicationRecord {
	app := ApplicationRecord{ID: appID}
	if openErr != nil {
		return app
	}
	if appFP, err := App().Fingerprint(ctx); err == nil {
		app.Fingerprint = appFingerprintPrefix + hex.EncodeToString(appFP.Digest[:])
	}
	return app
}

// Opened is the Store if the Run has opened one and not yet released it, or
// nil. It never opens the manifest.
func (s *ManifestSession) Opened() *ManifestStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store
}

// Application is the application record every Task commit carries.
func (s *ManifestSession) Application() ApplicationRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.app
}

// FirstMissWarning reports whether this is the first time the Run surfaces a
// store's safe cache-miss warning, so the Run states it once.
func (s *ManifestSession) FirstMissWarning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.missWarned
	s.missWarned = true
	return first
}

// FirstUnsavedWarning reports whether this is the first time the Run states
// its manifest was not saved, so a failed write is stated once.
func (s *ManifestSession) FirstUnsavedWarning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.unsaved
	s.unsaved = true
	return first
}

// Save waits until every record this Run committed or staged is on disk, so
// an Init+Finish caller that never calls Close still persists its history.
// Once the write succeeded it releases the Run's manifest lock, so a later
// Run on this workspace never waits for a Close this caller may not make; a
// later File reopens it. When the write failed it keeps the store and its
// lock, so Close retries the write (C30-085), and returns the failure. A
// release failure is returned and also kept for Close. Both already say what
// operation failed and on which path, so neither is wrapped again.
func (s *ManifestSession) Save() error {
	store := s.Opened()
	if store == nil {
		return nil
	}
	if flushErr := store.Flush(context.Background()); flushErr != nil {
		return flushErr
	}
	s.mu.Lock()
	if s.store == store {
		s.store, s.opened, s.openErr = nil, false, nil
	}
	s.mu.Unlock()
	releaseErr := store.Close()
	if releaseErr == nil {
		return nil
	}
	s.mu.Lock()
	s.finishErr = errors.Join(s.finishErr, releaseErr)
	s.mu.Unlock()
	return releaseErr
}

// Close ends the session: it releases the Run's exclusive manifest lock
// (spec §11.3) and returns any write or release failure. Already committed
// Task records on disk are unaffected; Close never rolls anything back.
func (s *ManifestSession) Close() error {
	s.mu.Lock()
	s.closed = true
	store, finishErr := s.store, s.finishErr
	s.mu.Unlock()
	return errors.Join(finishErr, store.Close())
}
