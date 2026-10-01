//go:build unix

package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func writeString(body string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, body)
		return err
	}
}

// entries lists dir's names.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = e.Name()
	}
	return names
}

func requireOnly(t *testing.T, dir string, want ...string) {
	t.Helper()
	got := entries(t, dir)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s holds %v, want exactly %v", dir, got, want)
	}
}

func inode(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Sys().(*syscall.Stat_t).Ino
}

func publishFile(t *testing.T, dest, body string, g Guard) error {
	t.Helper()
	s, err := StageFile(context.Background(), dest, 0o640, writeString(body))
	if err != nil {
		return err
	}
	return s.Commit(context.Background(), g)
}

func TestFileCommitReplacesAtomicallyAndLeavesNoStaging(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := inode(t, dest)
	var verified string
	g := Guard{Verify: func(_ context.Context, path string) error {
		b, err := os.ReadFile(path)
		verified = string(b)
		return err
	}}
	if err := publishFile(t, dest, "new", g); err != nil {
		t.Fatal(err)
	}
	if verified != "new" {
		t.Fatalf("Verify saw %q, want the committed bytes", verified)
	}
	if inode(t, dest) == before {
		t.Fatal("commit wrote in place; want a new inode by rename")
	}
	info, _ := os.Stat(dest)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
	requireOnly(t, dir, "f")
}

func TestSatisfiedRevalidationKeepsTheInode(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	if err := os.WriteFile(dest, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := inode(t, dest)
	g := Guard{Revalidate: func(context.Context, string) error { return ErrSatisfied }}
	if err := publishFile(t, dest, "same", g); err != nil {
		t.Fatal(err)
	}
	if inode(t, dest) != before {
		t.Fatal("a satisfied commit replaced the file")
	}
	requireOnly(t, dir, "f")
}

func TestFailedRevalidationPublishesNothing(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	errStale := errors.New("stale")
	g := Guard{Revalidate: func(context.Context, string) error { return errStale }}
	if err := publishFile(t, dest, "x", g); !errors.Is(err, errStale) {
		t.Fatalf("Commit = %v, want errStale", err)
	}
	requireOnly(t, dir)
}

func TestFailedFillLeavesNothingIncludingCreatedParents(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a", "b", "f")
	errFill := errors.New("fill failed")
	_, err := StageFile(context.Background(), dest, 0o644, func(io.Writer) error { return errFill })
	if !errors.Is(err, errFill) {
		t.Fatalf("StageFile = %v, want errFill", err)
	}
	requireOnly(t, dir)
}

func TestCancelledCommitPublishesNothing(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	s, err := StageFile(context.Background(), dest, 0o644, writeString("x"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Commit(ctx, Guard{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Commit(cancelled) = %v, want context.Canceled", err)
	}
	requireOnly(t, dir)
	if err := s.Commit(context.Background(), Guard{}); !errors.Is(err, ErrSpent) {
		t.Fatalf("second Commit = %v, want ErrSpent", err)
	}
}

func stageTree(t *testing.T, dest string, files map[string]string) *Staged {
	t.Helper()
	s, err := StageTree(context.Background(), dest, 0, func(_ context.Context, root string) error {
		for rel, body := range files {
			path := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTreeCommitReplacesWholesale(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg")
	if err := stageTree(t, dest, map[string]string{"old": "1", "keep/x": "2"}).Commit(context.Background(), Guard{}); err != nil {
		t.Fatal(err)
	}
	if err := stageTree(t, dest, map[string]string{"new": "3"}).Commit(context.Background(), Guard{}); err != nil {
		t.Fatal(err)
	}
	requireOnly(t, dest, "new")
	requireOnly(t, dir, "pkg")
	info, _ := os.Stat(dest)
	if info.Mode().Perm() != defaultTreeMode {
		t.Fatalf("tree root mode = %v, want %v", info.Mode().Perm(), defaultTreeMode)
	}
}

func TestTreeCommitOverAFileMovesItAside(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg")
	if err := os.WriteFile(dest, []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stageTree(t, dest, map[string]string{"a": "1"}).Commit(context.Background(), Guard{}); err != nil {
		t.Fatal(err)
	}
	requireOnly(t, dest, "a")
	requireOnly(t, dir, "pkg")
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	tree, file := filepath.Join(dir, "tree"), filepath.Join(dir, "file")
	if err := stageTree(t, tree, map[string]string{"d/a": "1"}).Commit(context.Background(), Guard{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{tree, file, filepath.Join(dir, "absent"), filepath.Join(dir, "no", "parent")} {
		if err := Remove(context.Background(), path, Guard{}); err != nil {
			t.Fatalf("Remove(%s): %v", path, err)
		}
	}
	requireOnly(t, dir)
}

// Concurrent commits to one destination all succeed, and the file always
// holds exactly one writer's whole content.
func TestConcurrentCommitsSerialize(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	const writers = 16
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Go(func() {
			body := strings.Repeat(fmt.Sprint(i%10), 4096)
			errs[i] = publishFile(t, dest, body, Guard{Verify: func(_ context.Context, path string) error {
				got, err := os.ReadFile(path)
				if err == nil && string(got) != body {
					err = fmt.Errorf("verify saw another writer's content under the lock")
				}
				return err
			}})
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	got, _ := os.ReadFile(dest)
	if len(got) != 4096 || strings.Count(string(got), string(got[0])) != 4096 {
		t.Fatalf("destination holds a mix of writers")
	}
	requireOnly(t, dir, "f")
}

// The lock is OS-backed: a second open descriptor (as another process
// would hold) cannot take it while a Hold is live, and can once released.
func TestLockIsCrossDescriptor(t *testing.T) {
	dir := t.TempDir()
	hold, err := Lock(context.Background(), filepath.Join(dir, "f"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if err := syscall.Flock(int(other.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		t.Fatal("another descriptor took the directory lock while it was held")
	}
	if err := hold.Release(); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(other.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	_ = syscall.Flock(int(other.Fd()), syscall.LOCK_UN)
	requireOnly(t, dir)
}

func TestLockWaitHonoursCancellation(t *testing.T) {
	dir := t.TempDir()
	other, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if err := syscall.Flock(int(other.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Lock(ctx, filepath.Join(dir, "f")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock while another descriptor holds it = %v, want DeadlineExceeded", err)
	}
	_ = syscall.Flock(int(other.Fd()), syscall.LOCK_UN)
	hold, err := Lock(context.Background(), filepath.Join(dir, "f"))
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	_ = hold.Release()
}

func TestIsStaging(t *testing.T) {
	name := filepath.Base(stagingName("/x"))
	if !IsStaging(name) || IsStaging("package.json") {
		t.Fatalf("IsStaging misclassified %q", name)
	}
}
