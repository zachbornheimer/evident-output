package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// taskManifestKey declares a Task named name on out and returns its stable
// manifest key (spec §3.1), computed at declaration — before the caller
// runs Define — the same way TestFileModeOnlySpecConsultsManifestOnSecondRun
// resolves a Task's manifest identity rather than assuming its display name
// doubles as its manifest key.
func taskManifestKey(t *testing.T, out *Output, name string) (*TaskHandle, string) {
	t.Helper()
	task := out.Task(name)
	out.mu.Lock()
	key, _, ok := out.taskManifestKeyLocked(task.id)
	out.mu.Unlock()
	if !ok {
		t.Fatalf("task %q was not declared", name)
	}
	return task, key
}

// TestOpaqueTaskDefinitionFallsBackToAppFingerprintWhenRunHasManifestActivity
// proves the ZYS-817 Decisions (2026-09-23) rule: "opaque Define/task
// definition identity automatically incorporates the application
// fingerprint as conservative fallback with zero caller code." A Run that
// already has manifest activity (here, an unrelated evo.File Task) commits
// a TaskRecord for a second, wholly opaque Task too, with zero code in that
// opaque Task's Define — and that record's own DefinitionFingerprint is
// non-empty.
func TestOpaqueTaskDefinitionFallsBackToAppFingerprintWhenRunHasManifestActivity(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")

	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })

	if err := runFileTask(t, out, "file", FileSpec{Path: path, Contents: []byte("desired")}); err != nil {
		t.Fatalf("file task: %v", err)
	}

	opaque, opaqueKey := taskManifestKey(t, out, "opaque")
	opaque.Define(func(ctx context.Context) error { return nil })
	if err := opaque.Wait(); err != nil {
		t.Fatalf("opaque task: %v", err)
	}
	_ = out.Close()

	store, err := manifest.Open(t.Context(), manifest.Config{StateDir: state}, manifest.NewOSEnvironment())
	if err != nil {
		t.Fatalf("reopen manifest: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	opaqueRecord, ok := store.Task(opaqueKey)
	if !ok {
		t.Fatal("opaque Task's TaskRecord was never committed")
	}
	if opaqueRecord.DefinitionFingerprint == "" {
		t.Fatal("opaque Task must automatically record the application-fingerprint fallback as its DefinitionFingerprint")
	}
	if len(opaqueRecord.Operations) != 0 {
		t.Fatalf("opaque Task recorded no File/Exec operations, want Operations empty, got %+v", opaqueRecord.Operations)
	}
}

// TestPreciseFileTaskLeavesTaskLevelDefinitionFingerprintEmpty proves the
// other half of the same rule: "precise File/Exec/Patch Basis beats the
// fallback" — a Task that recorded a precise tracked File operation gets no
// task-level application-fingerprint fallback at all, so it can never be
// invalidated by an unrelated application change the way an opaque Task's
// fallback deliberately can be.
func TestPreciseFileTaskLeavesTaskLevelDefinitionFingerprintEmpty(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")

	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })
	fileTask, fileKey := taskManifestKey(t, out, "file")
	fileTask.Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: path, Contents: []byte("desired")})
	})
	if err := fileTask.Wait(); err != nil {
		t.Fatalf("file task: %v", err)
	}
	_ = out.Close()

	store, err := manifest.Open(t.Context(), manifest.Config{StateDir: state}, manifest.NewOSEnvironment())
	if err != nil {
		t.Fatalf("reopen manifest: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	fileRecord, ok := store.Task(fileKey)
	if !ok {
		t.Fatal("file Task's TaskRecord was never committed")
	}
	if fileRecord.DefinitionFingerprint != "" {
		t.Fatalf("a Task with a precise tracked operation must not also carry the opaque application-fingerprint fallback, got %q", fileRecord.DefinitionFingerprint)
	}
	if len(fileRecord.Operations) != 1 {
		t.Fatalf("Operations = %+v, want exactly one", fileRecord.Operations)
	}
	for _, b := range fileRecord.Operations[0].Basis {
		if b.Kind == "app" {
			t.Fatalf("the automatic application-fingerprint fallback must never be injected into an operation's own user-visible Basis, got %+v", fileRecord.Operations[0].Basis)
		}
	}
}

// TestOpaqueOnlyRunNeverOpensManifest proves the fallback stays conservative
// in blast radius: a Run that never uses evo.File/evo.Exec/evo.Patch
// anywhere never opens or writes a manifest file at all, so a plain opaque
// consumer sees zero new filesystem side effects from this fallback.
func TestOpaqueOnlyRunNeverOpensManifest(t *testing.T) {
	state := t.TempDir()
	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("opaque-only")
	task.Define(func(ctx context.Context) error { return nil })
	if err := task.Wait(); err != nil {
		t.Fatalf("opaque task: %v", err)
	}
	_ = out.Close()

	if _, err := os.Stat(filepath.Join(state, "manifest-v1.json")); !os.IsNotExist(err) {
		t.Fatalf("a Run with no File/Exec/Patch anywhere must never create a manifest file, stat err = %v", err)
	}
}

// TestTaskOpaqueDefinitionFingerprintChangesWithAppFingerprint proves the
// fallback's own hash actually depends on the application fingerprint it
// falls back to, mirroring
// TestFileManifestAppBasisDriftForcesReconciliation's pure-function shape
// for evo.File's Basis-level App() case.
func TestTaskOpaqueDefinitionFingerprintChangesWithAppFingerprint(t *testing.T) {
	fpA := taskOpaqueDefinitionFingerprint("t1", "sha256:aaaa")
	fpB := taskOpaqueDefinitionFingerprint("t1", "sha256:bbbb")
	if fpA == fpB {
		t.Fatal("a changed application fingerprint must change the opaque Task's own DefinitionFingerprint")
	}
	fpSame := taskOpaqueDefinitionFingerprint("t1", "sha256:aaaa")
	if fpA != fpSame {
		t.Fatal("the same key and application fingerprint must reproduce the same DefinitionFingerprint")
	}
	fpOtherKey := taskOpaqueDefinitionFingerprint("t2", "sha256:aaaa")
	if fpA == fpOtherKey {
		t.Fatal("two different Task keys must not collide onto the same opaque DefinitionFingerprint")
	}
}

// TestOpaqueTaskDefinitionSkipsSecondRunsCallback proves the ZYS-817
// fallback actually gates a skip/rerun decision, not just storage: a Task
// with the same manifest key that stayed opaque (no File/Exec/Patch) on a
// prior Run, in a Run that already has other manifest activity, and whose
// application fingerprint has not changed, has its Define callback skipped
// entirely on the next Run — mirroring how a current File/Exec operation
// already skips re-running its own managed work.
//
// This test intentionally runs two sequential *Output Runs against the
// same on-disk manifest state (not two separate test cases, and not
// t.Parallel()) — that shared, single-test-function-scoped
// sharedRunStateDir/sharedManagedFilePath is the thing under test: whether
// Run 2 reuses Run 1's committed TaskRecord. Each is still its own unique
// t.TempDir(), isolated from every other test in this package.
func TestOpaqueTaskDefinitionSkipsSecondRunsCallback(t *testing.T) {
	sharedRunStateDir := t.TempDir()
	sharedManagedFilePath := filepath.Join(t.TempDir(), "managed.txt")

	// Run 1: an unrelated File Task opens the manifest, plus one opaque
	// Task, so the opaque Task's TaskRecord (Operations empty,
	// DefinitionFingerprint set) is committed for reuse.
	out1 := Init(Config{Isolated: true, StateDir: sharedRunStateDir})
	if err := runFileTask(t, out1, "file", FileSpec{Path: sharedManagedFilePath, Contents: []byte("desired")}); err != nil {
		t.Fatalf("run1 file task: %v", err)
	}
	run1Calls := 0
	opaque1 := out1.Task("opaque")
	opaque1.Define(func(ctx context.Context) error { run1Calls++; return nil })
	if err := opaque1.Wait(); err != nil {
		t.Fatalf("run1 opaque task: %v", err)
	}
	if err := out1.Close(); err != nil {
		t.Fatalf("run1 close: %v", err)
	}
	if run1Calls != 1 {
		t.Fatalf("run1 opaque Define calls = %d, want 1 (first Run has no prior record to skip by)", run1Calls)
	}

	// Run 2: same manifest key, same unrelated File Task (so the manifest
	// opens again), same application fingerprint (same test binary) — the
	// opaque Task's Define must be skipped this time.
	out2 := Init(Config{Isolated: true, StateDir: sharedRunStateDir})
	t.Cleanup(func() { _ = out2.Close() })
	if err := runFileTask(t, out2, "file", FileSpec{Path: sharedManagedFilePath, Contents: []byte("desired")}); err != nil {
		t.Fatalf("run2 file task: %v", err)
	}
	run2Calls := 0
	opaque2 := out2.Task("opaque")
	opaque2.Define(func(ctx context.Context) error { run2Calls++; return nil })
	if err := opaque2.Wait(); err != nil {
		t.Fatalf("run2 opaque task: %v", err)
	}
	if run2Calls != 0 {
		t.Fatal("an opaque Task's Define callback must be skipped on a Run whose application fingerprint and prior opaque TaskRecord both still match")
	}
}
