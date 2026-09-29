package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runFilesTask derives diff with Patch and commits it with Files from one
// Task, running between between the two calls. It returns the Files error.
func runFilesTask(t *testing.T, out *Output, diff string, between func()) error {
	t.Helper()
	var filesErr error
	task := out.Task("apply patch")
	task.Define(func(ctx context.Context) error {
		set, err := Patch(ctx, []byte(diff))
		if err != nil {
			filesErr = fmt.Errorf("Patch: %w", err)
			return filesErr
		}
		if between != nil {
			between()
		}
		filesErr = Files(ctx, set)
		return filesErr
	})
	_ = task.Wait()
	return filesErr
}

func readOrFatal(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// effectObjects lists the Object of every planned and committed Effect.
func effectObjects(out *Output) (planned, changed []string) {
	snap := out.Snapshot()
	for _, p := range snap.Plans {
		for _, r := range p.Records {
			planned = append(planned, r.Object)
		}
	}
	for _, c := range snap.Changes {
		for _, r := range c.Records {
			changed = append(changed, r.Object)
		}
	}
	return planned, changed
}

// twoFileEditDiff modifies greeting.txt and creates notes.txt.
const twoFileEditDiff = modifyGreetingDiff + `diff --git a/notes.txt b/notes.txt
new file mode 100644
--- /dev/null
+++ b/notes.txt
@@ -0,0 +1 @@
+note
`

func TestFilesRefusesToOverwriteStaleSource(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	path := filepath.Join(dir, "greeting.txt")
	err := runFilesTask(t, out, modifyGreetingDiff, func() {
		if writeErr := os.WriteFile(path, []byte("hello\nnewer\n"), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if !errors.Is(err, ErrStaleBasis) {
		t.Fatalf("Files = %v, want ErrStaleBasis", err)
	}
	if got := readOrFatal(t, path); got != "hello\nnewer\n" {
		t.Fatalf("stale source overwritten: %q", got)
	}
	if _, changed := effectObjects(out); len(changed) != 0 {
		t.Fatalf("a refused commit recorded changes: %v", changed)
	}
}

func TestFilesStaleCreationRefusesToOverwrite(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	notes := filepath.Join(dir, "notes.txt")
	err := runFilesTask(t, out, twoFileEditDiff, func() {
		if writeErr := os.WriteFile(notes, []byte("someone else\n"), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if !errors.Is(err, ErrStaleBasis) {
		t.Fatalf("Files = %v, want ErrStaleBasis", err)
	}
	if got := readOrFatal(t, notes); got != "someone else\n" {
		t.Fatalf("file created after derivation overwritten: %q", got)
	}
	// Not a transaction: the earlier file committed and its Effect stays.
	if got := readOrFatal(t, filepath.Join(dir, "greeting.txt")); got != "hello\nthere\n" {
		t.Fatalf("earlier file = %q, want committed", got)
	}
	if _, changed := effectObjects(out); len(changed) != 1 || changed[0] != "greeting.txt" {
		t.Fatalf("changes = %v, want only greeting.txt", changed)
	}
}

func TestFilesDryRunPlansWithoutMutation(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	out.mu.Lock()
	out.cfg.dryRun = true
	out.mu.Unlock()
	before := treeSnapshot(t, dir)
	if err := runFilesTask(t, out, twoFileEditDiff, nil); err != nil {
		t.Fatalf("Files: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, dir))
	planned, changed := effectObjects(out)
	if len(changed) != 0 {
		t.Fatalf("dry run recorded changes: %v", changed)
	}
	if len(planned) != 2 || planned[0] != "greeting.txt" || planned[1] != "notes.txt" {
		t.Fatalf("planned = %v, want greeting.txt and notes.txt", planned)
	}
}

func TestFilesAppliesAndRecordsOnlyChangedFiles(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	if err := runFilesTask(t, out, twoFileEditDiff, nil); err != nil {
		t.Fatalf("Files: %v", err)
	}
	if got := readOrFatal(t, filepath.Join(dir, "greeting.txt")); got != "hello\nthere\n" {
		t.Fatalf("greeting.txt = %q", got)
	}
	if got := readOrFatal(t, filepath.Join(dir, "notes.txt")); got != "note\n" {
		t.Fatalf("notes.txt = %q", got)
	}
	if _, changed := effectObjects(out); len(changed) != 2 {
		t.Fatalf("changes = %v, want both files", changed)
	}
}

func TestFilesUnchangedFileRecordsNoChange(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n", "tool.sh": "#!/bin/sh\n"})
	if err := os.Chmod(filepath.Join(dir, "tool.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	// tool.sh already has the mode this diff asks for; only greeting.txt changes.
	diff := modifyGreetingDiff + "diff --git a/tool.sh b/tool.sh\nold mode 100644\nnew mode 100755\n"
	if err := runFilesTask(t, out, diff, nil); err != nil {
		t.Fatalf("Files: %v", err)
	}
	if _, changed := effectObjects(out); len(changed) != 1 || changed[0] != "greeting.txt" {
		t.Fatalf("changes = %v, want only greeting.txt", changed)
	}
}

func TestFilesAlreadySatisfiedIsSuccessfulAndCompact(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	path := filepath.Join(dir, "greeting.txt")
	var first, second error
	task := out.Task("apply twice")
	task.Define(func(ctx context.Context) error {
		set, err := Patch(ctx, []byte(modifyGreetingDiff))
		if err != nil {
			return err
		}
		first = Files(ctx, set)
		second = Files(ctx, set)
		return errors.Join(first, second)
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("task: %v (first %v, second %v)", err, first, second)
	}
	if got := readOrFatal(t, path); got != "hello\nthere\n" {
		t.Fatalf("greeting.txt = %q", got)
	}
	if _, changed := effectObjects(out); len(changed) != 1 {
		t.Fatalf("changes = %v, want exactly one write", changed)
	}
}

func TestFilesSourceAlreadyAtDesiredStateIsSatisfied(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	path := filepath.Join(dir, "greeting.txt")
	err := runFilesTask(t, out, modifyGreetingDiff, func() {
		if writeErr := os.WriteFile(path, []byte("hello\nthere\n"), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if err != nil {
		t.Fatalf("Files = %v, want already satisfied", err)
	}
	if _, changed := effectObjects(out); len(changed) != 0 {
		t.Fatalf("changes = %v, want none", changed)
	}
}

// TestFilesClaimsEachPathWithoutCallerLocks proves Files holds each path
// itself: a writer already holding the path makes Files wait, and Files
// then sees that writer's change as stale instead of racing it. The
// caller takes no lock.
func TestFilesClaimsEachPathWithoutCallerLocks(t *testing.T) {
	dir, out := patchWorkspace(t, map[string]string{"greeting.txt": "hello\nworld\n"})
	path := filepath.Join(dir, "greeting.txt")
	derived, holding := make(chan struct{}), make(chan struct{})
	writer := out.Task("concurrent writer")
	writer.Define(func(ctx context.Context) error {
		<-derived
		spec := EffectSpec{Verb: EffectUpdate, Object: "greeting", Quantity: 1, Resource: FSResource(path)}
		return Effect(ctx, spec, func(context.Context) error {
			close(holding)
			time.Sleep(50 * time.Millisecond)
			return os.WriteFile(path, []byte("hello\nwriter\n"), 0o644)
		})
	})
	err := runFilesTask(t, out, modifyGreetingDiff, func() {
		close(derived)
		<-holding
	})
	_ = writer.Wait()
	if !errors.Is(err, ErrStaleBasis) {
		t.Fatalf("Files = %v, want ErrStaleBasis after waiting for the writer", err)
	}
	if got := readOrFatal(t, path); got != "hello\nwriter\n" {
		t.Fatalf("greeting.txt = %q, want the writer's change kept", got)
	}
}

func TestFilesRequiresTaskContext(t *testing.T) {
	if err := Files(context.Background(), FileSet{}); !errors.Is(err, ErrNoTaskContext) {
		t.Fatalf("err = %v, want ErrNoTaskContext", err)
	}
}
