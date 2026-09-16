package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store is one Run's exclusive handle on a single manifest file. Open
// acquires the lock; every other Run attempting the same manifest path
// blocks (or is cancelled) until Close releases it.
type Store struct {
	path string
	lock *fileLock
	doc  Document
	// missWarning is non-nil when Open found an existing manifest file it
	// could not trust (ErrCorrupt) — a safe cache miss, surfaced to the
	// caller instead of silently treated as "no history" with no signal.
	missWarning *Warning
}

// Open resolves cfg to a manifest path, acquires its exclusive lock
// (context-cancellable), and loads existing state if any. A missing file is
// an ordinary empty document, not a warning. A present-but-untrustworthy
// file is also an empty document, but Warning() reports why.
func Open(ctx context.Context, cfg Config, env Environment) (*Store, error) {
	path, err := Locate(cfg, env)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("manifest: create state dir for %q: %w", path, err)
	}
	lock, err := acquireLock(ctx, path+".lock")
	if err != nil {
		return nil, err
	}

	s := &Store{path: path, lock: lock}
	raw, readErr := os.ReadFile(path)
	switch {
	case os.IsNotExist(readErr):
		s.doc = newDocument(ApplicationRecord{})
	case readErr != nil:
		_ = lock.release()
		return nil, fmt.Errorf("manifest: read %q: %w", path, readErr)
	default:
		doc, decodeErr := decodeDocument(raw)
		if decodeErr != nil {
			s.missWarning = &Warning{Err: decodeErr}
			s.doc = newDocument(ApplicationRecord{})
		} else {
			s.doc = doc
		}
	}
	return s, nil
}

// Warning reports a safe cache-miss reason (corrupt/unknown prior manifest)
// discovered while opening the store, or nil when none occurred.
func (s *Store) Warning() error {
	if s == nil || s.missWarning == nil {
		return nil
	}
	return s.missWarning
}

// Operation returns the previously committed operation record at index i
// within taskKey's task, if this store has one that matches — the caller
// (evo.File) is responsible for comparing DefinitionFingerprint/Basis/
// Outputs against the current observation to decide freshness (spec
// §11.4: "matched by current semantic definition fingerprint ... within
// the Task").
func (s *Store) Operation(taskKey string, i int) (OperationRecord, bool) {
	task, ok := s.doc.Tasks[taskKey]
	if !ok || i < 0 || i >= len(task.Operations) {
		return OperationRecord{}, false
	}
	return task.Operations[i], true
}

// CommitTask atomically records task's full operation state as this Run's
// truth for taskKey, and persists app alongside it. Callers must only call
// CommitTask after the Task itself has fully succeeded (spec §11.3/§9.2) —
// this package has no notion of "the Task" and enforces nothing about when
// it is called; that ordering is the engine's responsibility.
func (s *Store) CommitTask(ctx context.Context, app ApplicationRecord, task TaskRecord) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("manifest: commit task %q: %w", task.Key, err)
	}
	s.doc.Application = app
	if s.doc.Tasks == nil {
		s.doc.Tasks = map[string]TaskRecord{}
	}
	s.doc.Tasks[task.Key] = task
	return s.writeAtomic()
}

// writeAtomic serializes the current document to a temp file in the same
// directory, fsyncs it, and renames it over the manifest path — the
// temp+fsync+rename contract spec §11.3 requires so a crash mid-write never
// leaves a half-written manifest.
func (s *Store) writeAtomic() error {
	raw, err := json.Marshal(s.doc)
	if err != nil {
		return fmt.Errorf("manifest: encode %q: %w", s.path, err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("manifest: create temp file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("manifest: write temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("manifest: fsync temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("manifest: close temp file %q: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("manifest: chmod temp file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("manifest: rename %q to %q: %w", tmpPath, s.path, err)
	}
	return nil
}

// Close releases this Store's exclusive lock. Already-committed Task
// records on disk are unaffected — Close never rolls anything back (spec
// §11.3: "cancellation/failure preserves already committed successful Task
// records").
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.lock.release()
}
