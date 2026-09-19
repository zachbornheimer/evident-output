package evo_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func writeGreeting(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runPatchTask(t *testing.T, out *evo.Output, name string, fn func(context.Context) error) error {
	t.Helper()
	var runErr error
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		runErr = fn(ctx)
		return runErr
	})
	_ = task.Wait()
	return runErr
}

func TestPatchMatrix_MutatesNothing(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	out := isolatedOutput(t, nil)
	err = runPatchTask(t, out, "patch", func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("Patch mutated %s", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello\nworld\n" {
		t.Fatalf("contents=%q err=%v", got, err)
	}
}

func TestPatchMatrix_OneFileDerivesContentsAndBasis(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	out := isolatedOutput(t, nil)
	var result evo.PatchResult
	err := runPatchTask(t, out, "patch", func(ctx context.Context) error {
		var err error
		result, err = evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("files=%d", len(result.Files))
	}
	spec := result.Files[0]
	if string(spec.Contents) != "hello\nthere\n" {
		t.Fatalf("contents=%q", spec.Contents)
	}
	if spec.Path != path {
		t.Fatalf("path=%q want %q", spec.Path, path)
	}
	if len(spec.Basis) != 1 {
		t.Fatalf("basis=%d", len(spec.Basis))
	}
	v, err := spec.Basis[0].Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.Key != path {
		t.Fatalf("Basis key=%q want %q", v.Key, path)
	}
}

func TestPatchMatrix_MultiFileRetainsPerPathProvenance(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := []byte("" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1 +1 @@\n" +
		"-one\n" +
		"+ONE\n" +
		"--- a/b.txt\n" +
		"+++ b/b.txt\n" +
		"@@ -1 +1 @@\n" +
		"-two\n" +
		"+TWO\n")
	out := isolatedOutput(t, nil)
	var result evo.PatchResult
	err := runPatchTask(t, out, "patch", func(ctx context.Context) error {
		var err error
		result, err = evo.Patch(ctx, evo.PatchSpec{Diff: diff, Dir: dir})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("files=%d", len(result.Files))
	}
	keys := map[string]string{}
	for _, spec := range result.Files {
		if len(spec.Basis) != 1 {
			t.Fatalf("path %s basis=%d", spec.Path, len(spec.Basis))
		}
		v, err := spec.Basis[0].Fingerprint(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		keys[spec.Path] = v.Key
	}
	if keys[a] != a || keys[b] != b {
		t.Fatalf("provenance=%v", keys)
	}
}

func TestPatchMatrix_DryRunPlansEffectsOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	out := evo.Init(evo.Config{
		Isolated: true, DryRun: true, StateDir: t.TempDir(),
		Plain: true, Color: evo.ColorNever, Stdout: io.Discard, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	err := runPatchTask(t, out, "apply", func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		if err != nil {
			return err
		}
		for _, spec := range result.Files {
			if err := evo.File(ctx, spec); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello\nworld\n" {
		t.Fatalf("dry-run wrote %q err=%v", got, err)
	}
	snap := out.Snapshot()
	found := false
	for _, p := range snap.Plans {
		for _, r := range p.Records {
			if r.Object == path {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("dry-run must plan an Effect for %q, plans=%+v", path, snap.Plans)
	}
}

func TestPatchMatrix_ApplyCommitsThroughFile(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	out := isolatedOutput(t, nil)
	err := runPatchTask(t, out, "apply", func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		if err != nil {
			return err
		}
		for _, spec := range result.Files {
			if err := evo.File(ctx, spec); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello\nthere\n" {
		t.Fatalf("got %q err=%v", got, err)
	}
	snap := out.Snapshot()
	found := false
	for _, c := range snap.Changes {
		for _, r := range c.Records {
			if r.Object == path {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("apply must record a committed Effect, changes=%+v", snap.Changes)
	}
}

func TestPatchMatrix_UnchangedDesiredIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	out := isolatedOutput(t, nil)
	var spec evo.FileSpec
	err := runPatchTask(t, out, "derive", func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		if err != nil {
			return err
		}
		spec = result.Files[0]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, spec.Contents, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	out2 := isolatedOutput(t, nil)
	err = runPatchTask(t, out2, "noop", func(ctx context.Context) error {
		return evo.File(ctx, spec)
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("already-matching File mutated the path")
	}
}

func TestPatchMatrix_StaleBasisRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	out := isolatedOutput(t, nil)
	var spec evo.FileSpec
	err := runPatchTask(t, out, "derive", func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		if err != nil {
			return err
		}
		spec = result.Files[0]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("C\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out2 := isolatedOutput(t, nil)
	err = runPatchTask(t, out2, "stale", func(ctx context.Context) error {
		return evo.File(ctx, spec)
	})
	if !errors.Is(err, evo.ErrStaleBasis) {
		t.Fatalf("err=%v, want ErrStaleBasis", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "C\n" {
		t.Fatalf("stale File overwrote C: %q err=%v", got, err)
	}
}

func TestPatchMatrix_MalformedLeavesFilesystemUntouched(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	diff := []byte("" +
		"--- a/hello.txt\n" +
		"+++ b/hello.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" hello\n" +
		"-nope\n" +
		"+there\n")
	out := isolatedOutput(t, nil)
	err = runPatchTask(t, out, "bad", func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{Diff: diff, Dir: dir})
		return err
	})
	if !errors.Is(err, evo.ErrPatchMalformed) {
		t.Fatalf("err=%v, want ErrPatchMalformed", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(before) {
		t.Fatalf("malformed Patch touched %q", got)
	}
}

func TestPatchMatrix_UnsupportedDeleteRename(t *testing.T) {
	dir := t.TempDir()
	out := isolatedOutput(t, nil)
	err := runPatchTask(t, out, "delete", func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{
			Diff: []byte("--- a/hello.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-hello\n"),
			Dir:  dir,
		})
		return err
	})
	if !errors.Is(err, evo.ErrPatchDelete) {
		t.Fatalf("delete=%v, want ErrPatchDelete", err)
	}
	err = runPatchTask(t, out, "rename", func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{
			Diff: []byte("" +
				"diff --git a/old.txt b/new.txt\n" +
				"rename from old.txt\n" +
				"rename to new.txt\n" +
				"--- a/old.txt\n" +
				"+++ b/new.txt\n" +
				"@@ -1 +1 @@\n" +
				"-a\n" +
				"+b\n"),
			Dir: dir,
		})
		return err
	})
	if !errors.Is(err, evo.ErrPatchRename) {
		t.Fatalf("rename=%v, want ErrPatchRename", err)
	}
}

func TestPatchMatrix_CancellationDoesNotInventManifest(t *testing.T) {
	dir := t.TempDir()
	state := t.TempDir()
	path := writeGreeting(t, dir)
	out := evo.Init(evo.Config{
		Isolated: true, StateDir: state,
		Plain: true, Color: evo.ColorNever, Stdout: io.Discard, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	err := runPatchTask(t, out, "cancel", func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		if err != nil {
			return err
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		return evo.File(canceled, result.Files[0])
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want canceled", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello\nworld\n" {
		t.Fatalf("canceled File wrote %q", got)
	}
	if _, err := os.Stat(filepath.Join(state, "manifest-v1.json")); err == nil {
		t.Fatal("canceled File invented manifest state")
	}
}

func TestPatchMatrix_NewFileFromDevNull(t *testing.T) {
	dir := t.TempDir()
	diff := []byte("" +
		"diff --git a/new.txt b/new.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/new.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+alpha\n" +
		"+beta\n")
	out := isolatedOutput(t, nil)
	err := runPatchTask(t, out, "create", func(ctx context.Context) error {
		result, err := evo.Patch(ctx, evo.PatchSpec{Diff: diff, Dir: dir})
		if err != nil {
			return err
		}
		return evo.File(ctx, result.Files[0])
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil || string(got) != "alpha\nbeta\n" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
