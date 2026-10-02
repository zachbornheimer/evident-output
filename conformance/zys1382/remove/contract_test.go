// Package remove_test is the ZYS-1382 execution contract for explicit
// removal: file.Remove(ctx) and tree.Remove(ctx) are the only way to ask
// for absence. Names and open-syntax choices:
// docs/zys-1382/contract-decisions.md.
package remove_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// contractRun runs fn inside one Task's Define callback on an isolated
// Output and returns fn's own error.
func contractRun(t *testing.T, cfg evo.Config, fn func(ctx context.Context) error) error {
	t.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = t.TempDir()
	}
	out := evo.Init(cfg)
	var (
		ran bool
		got error
	)
	task := out.Task("contract").Define(func(ctx context.Context) error {
		ran = true
		got = fn(ctx)
		return got
	})
	_ = task.Wait()
	_ = out.Finish()
	if !ran {
		t.Fatalf("contract Task callback never ran")
	}
	return got
}

// plant writes files (slash-separated relative path -> content) under root.
func plant(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

func TestFileRemoveDeletesTheFile(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"gone.txt": "bye", "kept.txt": "stay"})
	path := filepath.Join(dir, "gone.txt")
	if err := contractRun(t, evo.Config{}, evo.File{Path: path}.Remove); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, path) {
		t.Fatalf("Remove left the file in place")
	}
	if !exists(t, filepath.Join(dir, "kept.txt")) {
		t.Fatalf("Remove deleted a sibling")
	}
}

func TestTreeRemoveDeletesTheWholeTree(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "pkg")
	plant(t, root, map[string]string{"package.json": "{}", "lib/deep/a.js": "a", ".hidden/b": "b"})
	plant(t, dir, map[string]string{"sibling/kept.txt": "stay"})
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, root) {
		t.Fatalf("Remove left the tree in place")
	}
	if !exists(t, filepath.Join(dir, "sibling", "kept.txt")) {
		t.Fatalf("Remove deleted a sibling tree")
	}
}

func TestRemoveOfAnAbsentPathSucceeds(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	cases := []struct {
		name   string
		remove func(ctx context.Context) error
	}{
		{"file", evo.File{Path: missing}.Remove},
		{"tree", evo.Tree{Path: missing}.Remove},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := contractRun(t, evo.Config{}, tc.remove); err != nil {
				t.Fatalf("Remove of an absent path = %v, want nil", err)
			}
		})
	}
}

func TestRemoveIgnoresDeclaredContent(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"f.txt": "on disk"})
	path := filepath.Join(dir, "f.txt")
	f := evo.File{Path: path, Content: evo.Bytes("desired"), Mode: 0o600}
	if err := contractRun(t, evo.Config{}, f.Remove); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, path) {
		t.Fatalf("Remove on a File with Content left the file in place")
	}
}

func TestRemoveRefusesTheOtherCardinality(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"file.txt": "f", "tree/inner.txt": "t"})
	filePath, treePath := filepath.Join(dir, "file.txt"), filepath.Join(dir, "tree")
	cases := []struct {
		name   string
		remove func(ctx context.Context) error
		want   error
		keeps  string
	}{
		{"File.Remove on a directory", evo.File{Path: treePath}.Remove, evo.ErrFilePathTypeMismatch, filepath.Join(treePath, "inner.txt")},
		{"Tree.Remove on a regular file", evo.Tree{Path: filePath}.Remove, evo.ErrTreePathTypeMismatch, filePath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := contractRun(t, evo.Config{}, tc.remove); !errors.Is(err, tc.want) {
				t.Fatalf("Remove = %v, want %v", err, tc.want)
			}
			if !exists(t, tc.keeps) {
				t.Fatalf("a refused Remove still deleted %s", tc.keeps)
			}
		})
	}
}

func TestRemoveRequiresAPath(t *testing.T) {
	cases := []struct {
		name   string
		remove func(ctx context.Context) error
	}{
		{"file", evo.File{}.Remove},
		{"tree", evo.Tree{}.Remove},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := contractRun(t, evo.Config{}, tc.remove); !errors.Is(err, evo.ErrPathMissing) {
				t.Fatalf("Remove with empty Path = %v, want ErrPathMissing", err)
			}
		})
	}
}

// Nil Content is "no desired content declared", never "absent": Write
// refuses it and the path keeps what it held.
func TestWriteWithNilContentNeverMeansAbsence(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"file.txt": "kept", "tree/inner.txt": "kept"})
	filePath, treePath := filepath.Join(dir, "file.txt"), filepath.Join(dir, "tree")
	cases := []struct {
		name  string
		write func(ctx context.Context) error
		keeps string
	}{
		{"file", evo.File{Path: filePath}.Write, filePath},
		{"tree", evo.Tree{Path: treePath}.Write, filepath.Join(treePath, "inner.txt")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := contractRun(t, evo.Config{}, tc.write); !errors.Is(err, evo.ErrContentMissing) {
				t.Fatalf("Write with nil Content = %v, want ErrContentMissing", err)
			}
			got, err := os.ReadFile(tc.keeps)
			if err != nil || string(got) != "kept" {
				t.Fatalf("Write with nil Content disturbed %s: %q, %v", tc.keeps, got, err)
			}
		})
	}
}

// Empty Content is an empty file, not a removal.
func TestWriteWithEmptyContentNeverMeansAbsence(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"f.txt": "old"})
	path := filepath.Join(dir, "f.txt")
	for _, content := range []evo.FileContent{evo.Bytes([]byte(nil)), evo.Bytes("")} {
		if err := contractRun(t, evo.Config{}, evo.File{Path: path, Content: content}.Write); err != nil {
			t.Fatalf("Write: %v", err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Write with empty Content removed the file: %v", err)
		}
		if !info.Mode().IsRegular() || info.Size() != 0 {
			t.Fatalf("Write with empty Content left %v of size %d, want an empty regular file", info.Mode(), info.Size())
		}
	}
}

func TestRemoveUnderDryRunMutatesNothing(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"file.txt": "f", "tree/inner.txt": "t"})
	filePath, treePath := filepath.Join(dir, "file.txt"), filepath.Join(dir, "tree")
	err := contractRun(t, evo.Config{DryRun: true}, func(ctx context.Context) error {
		if err := (evo.File{Path: filePath}).Remove(ctx); err != nil {
			return err
		}
		return evo.Tree{Path: treePath}.Remove(ctx)
	})
	if err != nil {
		t.Fatalf("dry-run Remove: %v", err)
	}
	if !exists(t, filePath) || !exists(t, filepath.Join(treePath, "inner.txt")) {
		t.Fatalf("dry-run Remove deleted something")
	}
}

// Outside a Task nothing is mutated: Remove is a Task-scoped operation.
func TestRemoveOutsideATaskIsRefused(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"file.txt": "f", "tree/inner.txt": "t"})
	filePath, treePath := filepath.Join(dir, "file.txt"), filepath.Join(dir, "tree")
	if err := (evo.File{Path: filePath}).Remove(context.Background()); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("File.Remove outside a Task = %v, want ErrNoTaskContext", err)
	}
	if err := (evo.Tree{Path: treePath}).Remove(context.Background()); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Tree.Remove outside a Task = %v, want ErrNoTaskContext", err)
	}
	if !exists(t, filePath) || !exists(t, filepath.Join(treePath, "inner.txt")) {
		t.Fatalf("Remove outside a Task deleted something")
	}
}

// After Remove the path is observably absent through the rest of the shared
// vocabulary, and a declared Content no longer verifies.
func TestRemoveLeavesThePathAbsentToReadChecksumAndVerify(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"f.txt": "x", "tree/inner.txt": "y"})
	file := evo.File{Path: filepath.Join(dir, "f.txt"), Content: evo.Bytes("x")}
	tree := evo.Tree{Path: filepath.Join(dir, "tree")}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := file.Remove(ctx); err != nil {
			return err
		}
		if err := tree.Remove(ctx); err != nil {
			return err
		}
		if _, err := file.Read(ctx); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("File.Read after Remove = %v, want fs.ErrNotExist", err)
		}
		if _, err := file.Checksum(ctx); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("File.Checksum after Remove = %v, want fs.ErrNotExist", err)
		}
		if err := file.Verify(ctx); !errors.Is(err, evo.ErrVerifyMismatch) {
			t.Errorf("File.Verify after Remove = %v, want ErrVerifyMismatch", err)
		}
		if _, err := tree.Read(ctx); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Tree.Read after Remove = %v, want fs.ErrNotExist", err)
		}
		if _, err := tree.Checksum(ctx); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Tree.Checksum after Remove = %v, want fs.ErrNotExist", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Remove is idempotent and Write after Remove establishes content again.
func TestRemoveThenWriteReestablishes(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"f.txt": "old"})
	path := filepath.Join(dir, "f.txt")
	file := evo.File{Path: path, Content: evo.Bytes("new")}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		for range 2 {
			if err := file.Remove(ctx); err != nil {
				return err
			}
		}
		if err := file.Write(ctx); err != nil {
			return err
		}
		return file.Verify(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "new" {
		t.Fatalf("file after Remove+Write = %q, %v", got, err)
	}
}

// Removal takes exactly the destination: the parent directory stays and
// nothing (staging directory, trash, temporary) is left beside it.
func TestRemoveLeavesTheParentHoldingOnlyItsOtherEntries(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"f.txt": "f", "pkg/a/b.js": "b", "keep.txt": "k"})
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := (evo.File{Path: filepath.Join(dir, "f.txt")}).Remove(ctx); err != nil {
			return err
		}
		return evo.Tree{Path: filepath.Join(dir, "pkg")}.Remove(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "keep.txt" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("parent holds %v after Remove, want only [keep.txt]", names)
	}
}

// An empty directory is a tree; Tree.Remove deletes it.
func TestTreeRemoveDeletesAnEmptyDirectory(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: empty}.Remove); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if exists(t, empty) {
		t.Fatalf("Remove left the empty directory in place")
	}
}

// A refused Remove under DryRun is not required; but dry-run of an absent
// path still succeeds and creates nothing.
func TestRemoveUnderDryRunOfAbsentPathCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	err := contractRun(t, evo.Config{DryRun: true}, func(ctx context.Context) error {
		if err := (evo.File{Path: missing}).Remove(ctx); err != nil {
			return err
		}
		return evo.Tree{Path: missing}.Remove(ctx)
	})
	if err != nil {
		t.Fatalf("dry-run Remove of an absent path: %v", err)
	}
	if exists(t, missing) {
		t.Fatalf("dry-run Remove created %s", missing)
	}
}
