package manifest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreCommitTaskIsAtomicAndReadableAfterReopen(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{StateDir: dir}
	env := fakeEnvironment{}

	s, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	task := TaskRecord{Key: "t1", Operations: []OperationRecord{{
		Kind:                  "file",
		DefinitionFingerprint: "abc",
		Outputs:               []OutputRecord{{Kind: "file", Path: "out.txt", Digest: "deadbeef"}},
	}}}
	if err := s.CommitTask(context.Background(), ApplicationRecord{ID: "app"}, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if reopened.Warning() != nil {
		t.Fatalf("unexpected warning: %v", reopened.Warning())
	}
	op, ok := reopened.Operation("t1", 0)
	if !ok {
		t.Fatal("committed operation not found after reopen")
	}
	if op.DefinitionFingerprint != "abc" {
		t.Fatalf("DefinitionFingerprint = %q", op.DefinitionFingerprint)
	}
}

// TestStoreTaskReturnsWholeCommittedRecord proves Store.Task (ZYS-817) is
// symmetric with Store.Operation but returns the whole TaskRecord —
// including a Task-level DefinitionFingerprint that carries no Operations
// of its own, the opaque-Task shape commitManifestTaskLocked now commits.
func TestStoreTaskReturnsWholeCommittedRecord(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{StateDir: dir}
	env := fakeEnvironment{}

	s, err := Open(t.Context(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	opaque := TaskRecord{Key: "opaque", DefinitionFingerprint: "sha256:fallback"}
	if err := s.CommitTask(t.Context(), ApplicationRecord{ID: "app"}, opaque); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	got, ok := reopened.Task("opaque")
	if !ok {
		t.Fatal("committed opaque TaskRecord not found after reopen")
	}
	if got.DefinitionFingerprint != "sha256:fallback" {
		t.Fatalf("DefinitionFingerprint = %q, want %q", got.DefinitionFingerprint, "sha256:fallback")
	}
	if len(got.Operations) != 0 {
		t.Fatalf("Operations = %+v, want none", got.Operations)
	}
	if _, ok := reopened.Task("never-committed"); ok {
		t.Fatal("Task must report false for a key this store never committed")
	}
}

func TestStoreCorruptManifestIsSafeMissNotEvidence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), Config{StateDir: dir}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if s.Warning() == nil {
		t.Fatal("expected a warning for a corrupt manifest")
	}
	if !errors.Is(s.Warning(), ErrCorrupt) {
		t.Fatalf("warning = %v, want ErrCorrupt", s.Warning())
	}
	if _, ok := s.Operation("anything", 0); ok {
		t.Fatal("corrupt manifest must not report a prior operation as present")
	}
}

func TestStoreUnknownSchemaVersionIsSafeMiss(t *testing.T) {
	dir := t.TempDir()
	body := `{"schema_version": 999, "application": {"id":"x"}, "tasks": {}}`
	if err := os.WriteFile(filepath.Join(dir, manifestFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), Config{StateDir: dir}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if !errors.Is(s.Warning(), ErrCorrupt) {
		t.Fatalf("warning = %v, want ErrCorrupt for unknown schema version", s.Warning())
	}
}

func TestStoreConcurrentOpensSerializeOnTheLock(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{StateDir: dir}
	env := fakeEnvironment{}

	first, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}

	opened := make(chan *Store, 1)
	openErr := make(chan error, 1)
	go func() {
		second, err := Open(context.Background(), cfg, env)
		if err != nil {
			openErr <- err
			return
		}
		opened <- second
	}()

	// The second Open must not complete while the first still holds the
	// lock — give it a brief, bounded window to (wrongly) succeed anyway,
	// via a select against time.After rather than blocking the test
	// goroutine with a bare sleep.
	select {
	case <-opened:
		t.Fatal("second Open succeeded while first Store still held the lock")
	case err := <-openErr:
		t.Fatalf("second Open errored early: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case second := <-opened:
		_ = second.Close()
	case err := <-openErr:
		t.Fatalf("second Open errored after first released the lock: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("second Open never completed after first Store released the lock")
	}
}

func TestStoreOpenIsContextCancellable(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{StateDir: dir}
	env := fakeEnvironment{}

	first, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, cfg, env); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStorePreservesEarlierCommitsAfterACancelledLaterRun(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{StateDir: dir}
	env := fakeEnvironment{}

	s, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CommitTask(context.Background(), ApplicationRecord{ID: "app"}, TaskRecord{Key: "committed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s2.CommitTask(ctx, ApplicationRecord{ID: "app"}, TaskRecord{Key: "never-committed"}); err == nil {
		t.Fatal("CommitTask with a cancelled context should not succeed")
	}
	_ = s2.Close()

	s3, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s3.Close() }()
	if _, ok := s3.doc.Tasks["committed"]; !ok {
		t.Fatal("earlier committed task record was lost after a later cancelled commit attempt")
	}
	if _, ok := s3.doc.Tasks["never-committed"]; ok {
		t.Fatal("cancelled CommitTask must not have persisted its task record")
	}
}

func TestLocateWithStateDirBypassesDerivation(t *testing.T) {
	dir := t.TempDir()
	path, err := Locate(Config{StateDir: dir}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, manifestFileName) {
		t.Fatalf("path = %q", path)
	}
}

func TestLocateDerivesFromAppIDAndWorkspaceHash(t *testing.T) {
	cacheDir := t.TempDir()
	env := fakeEnvironment{cacheDir: cacheDir, exePath: "/usr/local/bin/mytool", moduleOK: true, modulePath: "example.com/mytool"}
	path, err := Locate(Config{Workspace: "/repo/one"}, env)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Locate(Config{Workspace: "/repo/two"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if path == other {
		t.Fatal("two distinct workspaces derived the same manifest path")
	}
	if filepath.Base(path) != manifestFileName {
		t.Fatalf("path = %q", path)
	}
}

func TestLocateAppIDOverride(t *testing.T) {
	cacheDir := t.TempDir()
	env := fakeEnvironment{cacheDir: cacheDir}
	path, err := Locate(Config{AppID: "custom-id", Workspace: "/repo"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(filepath.Dir(path))) != "custom-id" {
		t.Fatalf("path = %q, want custom-id path segment", path)
	}
}

type fakeEnvironment struct {
	cacheDir   string
	cacheErr   error
	exePath    string
	exeErr     error
	modulePath string
	moduleOK   bool
}

func (f fakeEnvironment) Executable() (string, error)   { return f.exePath, f.exeErr }
func (f fakeEnvironment) UserCacheDir() (string, error) { return f.cacheDir, f.cacheErr }
func (f fakeEnvironment) ReadBuildInfo() (string, bool) { return f.modulePath, f.moduleOK }

// TestStoreStageTaskWritesOnlyOnFlush proves a staged record stays in
// memory until Flush (or the next CommitTask) writes it, and that Flush
// with nothing staged writes nothing.
func TestStoreStageTaskWritesOnlyOnFlush(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{StateDir: dir}
	env := fakeEnvironment{}
	s, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	path, err := Locate(cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Flush with nothing staged wrote %s (stat err %v)", path, err)
	}
	s.StageTask(ApplicationRecord{ID: "app"}, TaskRecord{Key: "opaque", DefinitionFingerprint: "sha256:fallback"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("StageTask wrote %s before Flush (stat err %v)", path, err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), cfg, env)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if got, ok := reopened.Task("opaque"); !ok || got.DefinitionFingerprint != "sha256:fallback" {
		t.Fatalf("staged record after Flush = %+v, %v", got, ok)
	}
}

// TestStoreCommitTaskNeverWaitsOnTheDiskAndCoalesces proves CommitTask
// returns while a write is still in flight, and that every commit made
// during that write lands in one more write rather than one each.
func TestStoreCommitTaskNeverWaitsOnTheDiskAndCoalesces(t *testing.T) {
	s, err := Open(t.Context(), Config{StateDir: t.TempDir()}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var writes atomic.Int32
	s.write = func([]byte) error {
		if writes.Add(1) == 1 {
			<-release
		}
		return nil
	}
	const commits = 100
	committed := make(chan struct{})
	go func() {
		for i := range commits {
			_ = s.CommitTask(t.Context(), ApplicationRecord{ID: "app"}, TaskRecord{Key: fmt.Sprint(i)})
		}
		close(committed)
	}()
	select {
	case <-committed:
	case <-time.After(5 * time.Second):
		t.Fatal("CommitTask waited on a write in flight")
	}
	close(release)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := writes.Load(); got > 2 {
		t.Fatalf("%d commits during one write cost %d writes, want at most 2", commits, got)
	}
}

// TestStoreFlushReportsAWriteFailure proves a failed write reaches the
// caller through Flush and Close instead of being dropped.
func TestStoreFlushReportsAWriteFailure(t *testing.T) {
	s, err := Open(t.Context(), Config{StateDir: t.TempDir()}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	disk := errors.New("disk full")
	s.write = func([]byte) error { return disk }
	if err := s.CommitTask(t.Context(), ApplicationRecord{ID: "app"}, TaskRecord{Key: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(t.Context()); !errors.Is(err, disk) {
		t.Fatalf("Flush() = %v, want %v", err, disk)
	}
	if err := s.Close(); !errors.Is(err, disk) {
		t.Fatalf("Close() = %v, want %v", err, disk)
	}
}

// TestStoreFlushRetriesAWriteThatFailedOnce proves one transient write
// failure does not lose the Run's history: the next Flush (here, Close's)
// makes a fresh attempt instead of handing back the stale error.
func TestStoreFlushRetriesAWriteThatFailedOnce(t *testing.T) {
	cfg := Config{StateDir: t.TempDir()}
	s, err := Open(t.Context(), cfg, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	transient := errors.New("transient")
	var writes atomic.Int32
	s.write = func(raw []byte) error {
		if writes.Add(1) == 1 {
			return transient
		}
		return s.writeAtomic(raw)
	}
	if err := s.CommitTask(t.Context(), ApplicationRecord{ID: "app"}, TaskRecord{Key: "t"}); err != nil {
		t.Fatal(err)
	}
	// The background write may or may not have run yet; either way the
	// first attempt fails and Close's Flush must try again.
	for writes.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil after a retried write (writes=%d)", err, writes.Load())
	}
	reopened, err := Open(t.Context(), cfg, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if _, ok := reopened.Task("t"); !ok {
		t.Fatal(`Task("t") missing after reopen: the retried write never reached disk`)
	}
}
