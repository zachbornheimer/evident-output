// Package patch_test binds contract §28 (external fixers) and §30 "Patch
// and Files" rules that no existing test proves to the public evo API.
package patch_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	originalText = "hello\nworld\n"
	fixedText    = "hello\nthere\n"
	settledText  = "hello\nthere\nagain\n"

	helloToThere = "--- a/greeting.txt\n+++ b/greeting.txt\n@@ -1,2 +1,2 @@\n hello\n-world\n+there\n"
	thereAgain   = "--- a/greeting.txt\n+++ b/greeting.txt\n@@ -1,2 +1,3 @@\n hello\n there\n+again\n"
)

// workspace writes files into a fresh directory, makes it the working
// directory (Patch resolves diff paths against it), and returns the
// directory and an Output whose manifest lives outside it.
func workspace(t *testing.T, files map[string]string) (string, *evo.Output) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	return dir, out
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func runTask(t *testing.T, out *evo.Output, name string, body func(context.Context) error) error {
	t.Helper()
	var bodyErr error
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		bodyErr = body(ctx)
		return bodyErr
	})
	_ = task.Wait()
	return bodyErr
}

func changedRecordCount(out *evo.Output) int {
	count := 0
	for _, changes := range out.Snapshot().Changes {
		count += len(changes.Records)
	}
	return count
}

func TestC28_001_NativeDiffFlowCommitsThroughFile(t *testing.T) {
	dir, out := workspace(t, map[string]string{"greeting.txt": originalText})
	path := filepath.Join(dir, "greeting.txt")

	err := runTask(t, out, "fix greeting", func(ctx context.Context) error {
		set, err := evo.Patch(ctx, []byte(helloToThere))
		if err != nil {
			return err
		}
		return evo.Files(ctx, set)
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := read(t, path); got != fixedText {
		t.Fatalf("file after Patch = %q, want %q", got, fixedText)
	}
	if got := changedRecordCount(out); got != 1 {
		t.Fatalf("changed Effects = %d, want 1 recorded by Patch", got)
	}
}

func TestC28_002_MultiPassConvergenceStaysOneTask(t *testing.T) {
	dir, out := workspace(t, map[string]string{"greeting.txt": originalText})
	const taskName = "stabilize Go source"

	err := runTask(t, out, taskName, func(ctx context.Context) error {
		for _, diff := range []string{helloToThere, thereAgain} {
			set, err := evo.Patch(ctx, []byte(diff))
			if err != nil {
				return err
			}
			if err := evo.Files(ctx, set); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("two passes: %v", err)
	}
	if got := read(t, filepath.Join(dir, "greeting.txt")); got != settledText {
		t.Fatalf("file after convergence = %q, want %q", got, settledText)
	}
	tasks := out.Snapshot().Tasks
	if len(tasks) != 1 || tasks[0].Name != taskName {
		t.Fatalf("snapshot tasks = %+v, want the single task %q", tasks, taskName)
	}
}

func TestC30_071_PatchIsAllOrNothingAndNeverOverwritesAConcurrentEdit(t *testing.T) {
	const twoFileDiff = "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-one\n+uno\n" +
		"--- a/b.txt\n+++ b/b.txt\n@@ -1,1 +1,1 @@\n-two\n+dos\n"
	const interloperText = "changed by someone else\n"
	dir, out := workspace(t, map[string]string{"a.txt": "one\n", "b.txt": interloperText})

	err := runTask(t, out, "commit both", func(ctx context.Context) error {
		_, err := evo.Patch(ctx, []byte(twoFileDiff))
		return err
	})
	if !errors.Is(err, evo.ErrPatchDoesNotApply) {
		t.Fatalf("Patch = %v, want ErrPatchDoesNotApply for the file that changed", err)
	}
	if got := read(t, filepath.Join(dir, "b.txt")); got != interloperText {
		t.Fatalf("b.txt = %q, want the newer state left untouched", got)
	}
	if got := read(t, filepath.Join(dir, "a.txt")); got != "one\n" {
		t.Fatalf("a.txt = %q, want it untouched: applicability is validated before any commit", got)
	}
	if got := changedRecordCount(out); got != 0 {
		t.Fatalf("changed Effects = %d, want 0 when nothing committed", got)
	}
}
