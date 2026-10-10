package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// Store is one Run's exclusive handle on a single manifest file. Open
// acquires the lock; every other Run attempting the same manifest path
// blocks (or is cancelled) until Close releases it.
//
// Records are held in memory and written by one background writer that
// coalesces every record put since its last write into a single
// temp+fsync+rename, so a caller recording a Task never waits on the disk.
// Flush and Close wait for the writer and report its failure.
type Store struct {
	path string
	lock *sysfs.FileLock
	// missWarning is non-nil when Open found an existing manifest file it
	// could not trust (ErrCorrupt) — a safe cache miss, surfaced to the
	// caller instead of silently treated as "no history" with no signal.
	missWarning *Warning
	// write persists one encoded document; writeAtomic unless a test
	// substitutes it.
	write func(raw []byte) error

	mu      sync.Mutex
	settled *sync.Cond
	doc     Document
	w       writerState
}

// writerState tracks the document's versions against what is on disk.
// Every put bumps version; requested is the newest version a write was
// asked for; written is the newest version on disk; attempted is the newest
// version the writer tried, and failed holds that try's error.
type writerState struct {
	version, requested, written, attempted uint64
	running                                bool
	failed                                 error
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
	if err := sysfs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("manifest: create state dir for %q: %w", path, err)
	}
	lock, err := sysfs.AcquireFileLock(ctx, path+".lock")
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	s := newStore(path, lock)
	raw, readErr := sysfs.ReadFile(path)
	switch {
	case errors.Is(readErr, fs.ErrNotExist):
		s.doc = newDocument(ApplicationRecord{})
	case readErr != nil:
		_ = lock.Release()
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

func newStore(path string, lock *sysfs.FileLock) *Store {
	s := &Store{path: path, lock: lock}
	s.write = s.writeAtomic
	s.settled = sync.NewCond(&s.mu)
	return s
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
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.doc.Tasks[taskKey]
	if !ok || i < 0 || i >= len(task.Operations) {
		return OperationRecord{}, false
	}
	return task.Operations[i], true
}

// Task returns the previously committed TaskRecord for taskKey, if this
// store has one — symmetric with Operation, but returning the whole record
// (including its own DefinitionFingerprint, ZYS-817) rather than one
// operation within it.
func (s *Store) Task(taskKey string) (TaskRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.doc.Tasks[taskKey]
	return task, ok
}

// CommitTask records task's full operation state as this Run's truth for
// taskKey, persists app alongside it, and hands the write to the background
// writer without waiting for it. Flush or Close makes it durable and
// reports a write failure. Callers must only call CommitTask after the Task
// itself has fully succeeded (spec §11.3/§9.2) — this package has no notion
// of "the Task" and enforces nothing about when it is called; that ordering
// is the engine's responsibility.
func (s *Store) CommitTask(ctx context.Context, app ApplicationRecord, task TaskRecord) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("manifest: commit task %q: %w", task.Key, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putLocked(app, task)
	s.requestWriteLocked()
	return nil
}

// StageTask records task like CommitTask, but asks for no write: the next
// CommitTask or Flush writes it. It is for records nothing reads back
// within the Run, so a Run of N such Tasks pays one write, not N.
func (s *Store) StageTask(app ApplicationRecord, task TaskRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putLocked(app, task)
}

// Flush waits until every record put so far is on disk, writing any that
// are only staged, and returns the write's error when it failed. It does
// nothing when nothing is pending. Each Flush makes a fresh attempt at a
// version the writer already gave up on, so one transient failure (EINTR,
// a brief ENOSPC) costs a retry, not the Run's history.
func (s *Store) Flush(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	target := s.w.version
	if s.w.written >= target {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("manifest: flush %q: %w", s.path, err)
	}
	if !s.w.running {
		s.w.attempted = s.w.written
	}
	s.requestWriteLocked()
	for s.w.written < target && (s.w.running || s.w.attempted < target) {
		s.settled.Wait()
	}
	if s.w.written >= target {
		return nil
	}
	return s.w.failed
}

func (s *Store) putLocked(app ApplicationRecord, task TaskRecord) {
	s.doc.Application = app
	if s.doc.Tasks == nil {
		s.doc.Tasks = map[string]TaskRecord{}
	}
	s.doc.Tasks[task.Key] = task
	s.w.version++
}

// requestWriteLocked asks the writer for the current version, starting it
// when it is idle. A running writer picks the request up after its current
// write, so a burst of commits costs one more write, not one each.
func (s *Store) requestWriteLocked() {
	s.w.requested = s.w.version
	if s.w.running {
		return
	}
	s.w.running = true
	go s.writeLoop()
}

// writeLoop writes the newest version until nothing requested is missing
// from disk. It gives up on a version after one failed try; the failure
// stays in failed until a later write succeeds, and the next Flush retries.
func (s *Store) writeLoop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.w.written < s.w.requested && s.w.attempted < s.w.requested {
		version := s.w.version
		raw, err := json.Marshal(s.doc)
		if err == nil {
			s.mu.Unlock()
			err = s.write(raw)
			s.mu.Lock()
		} else {
			err = fmt.Errorf("manifest: encode %q: %w", s.path, err)
		}
		s.w.attempted = version
		if err != nil {
			s.w.failed = err
		} else {
			s.w.written, s.w.failed = version, nil
		}
		s.settled.Broadcast()
	}
	s.w.running = false
	s.settled.Broadcast()
}

// writeAtomic writes raw, an encoded document, to a temp file in the same
// directory, fsyncs it, and renames it over the manifest path — the
// temp+fsync+rename contract spec §11.3 requires so a crash mid-write never
// leaves a half-written manifest.
func (s *Store) writeAtomic(raw []byte) error {
	dir := filepath.Dir(s.path)
	tmp, err := sysfs.CreateTemp(dir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("manifest: create temp file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = sysfs.Remove(tmpPath)
		return fmt.Errorf("manifest: write temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = sysfs.Remove(tmpPath)
		return fmt.Errorf("manifest: fsync temp file %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = sysfs.Remove(tmpPath)
		return fmt.Errorf("manifest: close temp file %q: %w", tmpPath, err)
	}
	if err := sysfs.Chmod(tmpPath, 0o600); err != nil {
		_ = sysfs.Remove(tmpPath)
		return fmt.Errorf("manifest: chmod temp file %q: %w", tmpPath, err)
	}
	if err := sysfs.Rename(tmpPath, s.path); err != nil {
		_ = sysfs.Remove(tmpPath)
		return fmt.Errorf("manifest: rename %q to %q: %w", tmpPath, s.path, err)
	}
	return nil
}

// Close flushes every pending record, then releases this Store's
// exclusive lock, and returns both errors. Already-committed Task records
// on disk are unaffected — Close never rolls anything back (spec §11.3:
// "cancellation/failure preserves already committed successful Task
// records").
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return errors.Join(s.Flush(context.Background()), s.lock.Release())
}
