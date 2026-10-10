package engine

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestOpaqueTaskDefinitionRunsEveryRun proves an opaque Task (no Verify,
// no File/Exec operation) runs its Define callback on every Run, even when
// a prior Run of the same binary committed a TaskRecord for it. Evo cannot
// observe opaque state (a push, an API call), so application identity can
// never prove it current. The File Task first opens the manifest, the shape
// that used to trigger the skip.
func TestOpaqueTaskDefinitionRunsEveryRun(t *testing.T) {
	stateDir := t.TempDir()
	managed := filepath.Join(t.TempDir(), "managed.txt")

	runOnce := func(run int) int {
		out := Init(Config{Isolated: true, StateDir: stateDir})
		defer func() { _ = out.Close() }()
		if err := runFileTask(t, out, "file", FileSpec{Path: managed, Contents: []byte("desired")}); err != nil {
			t.Fatalf("run %d file task: %v", run, err)
		}
		pushes := 0
		push := out.Task("push branch")
		push.Define(func(ctx context.Context) error {
			return Effect(ctx, EffectSpec{Verb: EffectPush, Object: "branch", Quantity: 1}, func(context.Context) error {
				pushes++
				return nil
			})
		})
		if err := push.Wait(); err != nil {
			t.Fatalf("run %d push task: %v", run, err)
		}
		out.mu.Lock()
		got := out.taskByRef[push.id].rec.Resolution()
		out.mu.Unlock()
		if got != ResolutionExecuted {
			t.Fatalf("run %d push resolution = %q, want %q", run, got, ResolutionExecuted)
		}
		return pushes
	}

	for run := 1; run <= 2; run++ {
		if got := runOnce(run); got != 1 {
			t.Fatalf("run %d Effect callback calls = %d, want 1: an opaque Task must never be skipped as already satisfied", run, got)
		}
	}
}

// TestOpaqueTasksDoNotRewriteTheManifestPerSettle proves opaque Tasks in a
// Run with manifest activity cost no manifest write each: before Close the
// file on disk holds only the File Task's record, however many opaque
// Tasks settled after it. Each opaque settle used to marshal, fsync, and
// rename the whole manifest under o.mu (400 no-op Tasks: 3.6ms without a
// File Task, 5.98s with one). Finish (here via Close) writes the staged
// records once.
func TestOpaqueTasksDoNotRewriteTheManifestPerSettle(t *testing.T) {
	const opaqueTasks = 50
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")
	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })

	if err := runFileTask(t, out, "file", FileSpec{Path: path, Contents: []byte("desired")}); err != nil {
		t.Fatalf("file task: %v", err)
	}
	// The File Task's commit is written in the background; wait for it so
	// the file on disk is the baseline the opaque settles must not touch.
	if err := out.manifestStore.Flush(t.Context()); err != nil {
		t.Fatalf("flush file task record: %v", err)
	}
	group := out.Group("opaque")
	for i := range opaqueTasks {
		group.Task(fmt.Sprintf("opaque %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("opaque tasks: %v", err)
	}

	if got := manifestTaskCount(t, state); got != 1 {
		t.Fatalf("manifest on disk holds %d task records before Close, want 1 (the File Task): opaque settles must not rewrite it", got)
	}
	_ = out.Close()
	if got := manifestTaskCount(t, state); got != 1+opaqueTasks {
		t.Fatalf("manifest after Close holds %d task records, want %d: Close must write every staged opaque record", got, 1+opaqueTasks)
	}
}

func manifestTaskCount(t *testing.T, state string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(state, "manifest-v1.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var doc manifest.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return len(doc.Tasks)
}
