//go:build v06acceptance

// Package pending_test holds increment 2+'s behavioral fixtures: File,
// dry-run-with-File, cancellation-before-mutation, and opaque-callback
// manifest re-entry (§8, §2.1). None of it exists in this worktree yet
// (increment 1's blast radius explicitly excludes File/Exec/fingerprints/
// manifest work) — deliberately red-by-compile under
// `go test -tags v06acceptance ./conformance/future/behavior/pending`
// until a later increment implements evo.File/evo.FileSpec. A separate Go
// package from ../behavior_test.go so this package's compile failure never
// blocks that one's tests from running.
package pending_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func isolated(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: os.Stdout, Stderr: os.Stderr})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestV06FileCreateAndUnchangedUseFilesystemEvidence(t *testing.T) {
	out := isolated(t)
	path := filepath.Join(t.TempDir(), "managed.txt")
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("file-create")
		task.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("desired")})
		})
		return nil
	})
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	second := isolated(t)
	second.Run(context.Background(), func(ctx context.Context) error {
		task := second.Task("file-create")
		task.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("desired")})
		})
		return nil
	})
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("unchanged File replaced or changed mtime: before=%v after=%v", before, after)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "desired" {
		t.Fatalf("file = %q, err=%v", got, err)
	}
}

func TestV06FileReconcilesChangedContentsAndDrift(t *testing.T) {
	out := isolated(t)
	path := filepath.Join(t.TempDir(), "managed.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("file-repair")
		task.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("new")})
		})
		return nil
	})
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("file = %q, want new", got)
	}
}

func TestV06FileNilAndEmptyContentsAreDistinct(t *testing.T) {
	out := isolated(t)
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.txt")
	nilPath := filepath.Join(dir, "nil.txt")
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("empty")
		task.Define(func(ctx context.Context) error {
			if err := evo.File(ctx, evo.FileSpec{Path: emptyPath, Contents: []byte{}}); err != nil {
				return err
			}
			return evo.File(ctx, evo.FileSpec{Path: nilPath, Contents: nil})
		})
		return nil
	})
	got, err := os.ReadFile(emptyPath)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty file = %q, err=%v", got, err)
	}
	if _, err := os.Stat(nilPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nil contents created file: %v", err)
	}
}

func TestV06FileRejectsSymlink(t *testing.T) {
	out := isolated(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	var fileErr error
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("symlink")
		task.Define(func(ctx context.Context) error {
			fileErr = evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("unsafe")})
			return fileErr
		})
		return nil
	})
	if fileErr == nil {
		t.Fatal("File on symlink must return an error")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "safe" {
		t.Fatalf("symlink target = %q, err=%v", got, err)
	}
}

func TestV06FileNoTaskContextAndClosedContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outside.txt")
	if err := evo.File(context.Background(), evo.FileSpec{Path: path, Contents: []byte("no")}); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("File outside task = %v", err)
	}
	out := isolated(t)
	var callbackCtx context.Context
	out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("closed")
		task.Define(func(ctx context.Context) error { callbackCtx = ctx; return nil })
		return nil
	})
	if err := evo.File(callbackCtx, evo.FileSpec{Path: path, Contents: []byte("late")}); !errors.Is(err, evo.ErrTaskClosed) {
		t.Fatalf("File after task = %v", err)
	}
}

func TestV06CancellationBeforeMutationDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancel.txt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := isolated(t)
	out.Run(ctx, func(ctx context.Context) error {
		task := out.Task("cancelled")
		task.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("no")})
		})
		return nil
	})
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled File created path: %v", err)
	}
	// A cancelled Run must not be reported as if it settled successfully —
	// otherwise this test could pass vacuously even if Run never attempted
	// the Task at all.
	if out.Err() == nil {
		t.Fatal("cancelled Run must report a non-nil error, not silent success")
	}
}

// TestV06DryRunDoesNotWriteButAdvancesSuccessfully proves BOTH halves of the
// dry-run contract: no mutation occurs, AND the Task still settles as a
// success (the run reports no error) rather than silently doing nothing or
// failing — dry-run is a planning mode, not a no-op mode that leaves the
// Task in an unresolved or failed state.
func TestV06DryRunDoesNotWriteButAdvancesSuccessfully(t *testing.T) {
	dry := evo.Init(evo.Config{Isolated: true, DryRun: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = dry.Close() })
	dryPath := filepath.Join(t.TempDir(), "dry.txt")
	entered := false
	dry.Run(context.Background(), func(ctx context.Context) error {
		dryTask := dry.Task("dry")
		dryTask.Define(func(ctx context.Context) error {
			entered = true
			return evo.File(ctx, evo.FileSpec{Path: dryPath, Contents: []byte("no")})
		})
		return nil
	})
	if !entered {
		t.Fatal("dry-run must still enter Define for read-only discovery/planning")
	}
	if _, err := os.Stat(dryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run File created path: %v", err)
	}
	if dry.Err() != nil {
		t.Fatalf("dry-run Task must advance/settle successfully, got err=%v", dry.Err())
	}
}

// TestV06OpaqueCallbackReentersWithPriorTrackedManifest proves §2.1's
// soundness rule in two explicit stages, not one aggregate count: first that
// run 1 actually succeeded and persisted real tracked state (proving the
// manifest that run 2 will see is genuine, not absent-by-accident), then that
// run 2 — same StateDir, same application fingerprint — still re-enters the
// opaque Define callback rather than trusting the prior manifest alone.
func TestV06OpaqueCallbackReentersWithPriorTrackedManifest(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "tracked.txt")
	called := 0
	run := func(out *evo.Output) {
		out.Run(context.Background(), func(ctx context.Context) error {
			task := out.Task("opaque")
			task.Define(func(ctx context.Context) error {
				called++
				return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("same")})
			})
			return nil
		})
	}

	first := evo.Init(evo.Config{Isolated: true, StateDir: state})
	run(first)
	firstErr := first.Err()
	_ = first.Close()
	if called != 1 {
		t.Fatalf("first run Define calls = %d, want 1", called)
	}
	if firstErr != nil {
		t.Fatalf("first run must succeed and persist real state, got err=%v", firstErr)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "same" {
		t.Fatalf("first run did not persist tracked file: contents=%q err=%v", got, err)
	}

	second := evo.Init(evo.Config{Isolated: true, StateDir: state})
	run(second)
	secondErr := second.Err()
	_ = second.Close()
	if called != 2 {
		t.Fatal("second run must re-enter the opaque Define callback despite a prior successful manifest")
	}
	if secondErr != nil {
		t.Fatalf("second run must also settle successfully, got err=%v", secondErr)
	}
}
