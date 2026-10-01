package remove_test

// Adversarial hardening tests for File.Remove and Tree.Remove. Each test is
// named for the weakness it guards; sources are in
// docs/zys-1382/adversarial-research.md.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func advRun(tb testing.TB, cfg evo.Config, fn func(context.Context) error) error {
	tb.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = tb.TempDir()
	}
	out := evo.Init(cfg)
	defer func() { _ = out.Close() }()
	err := out.Task("adversarial").Define(fn).Wait()
	_ = out.Finish()
	return err
}

func advPlant(tb testing.TB, root string, rels ...string) {
	tb.Helper()
	for _, rel := range rels {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

func advPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s should still exist: %v", path, err)
	}
}

func advGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s should be gone (stat err %v)", path, err)
	}
}

// Rust CVE-2022-21658 class: recursive removal must unlink a symlinked
// directory, never descend into it.
func TestAdversarial_SymlinkedDirectoryInsideTreeNotFollowed(t *testing.T) {
	root, outside := filepath.Join(t.TempDir(), "node_modules"), t.TempDir()
	advPlant(t, root, "a/index.js")
	advPlant(t, outside, "precious.txt")
	if err := os.Symlink(outside, filepath.Join(root, "a", "linked")); err != nil {
		t.Fatal(err)
	}
	if err := advRun(t, evo.Config{}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatal(err)
	}
	advGone(t, root)
	advPresent(t, filepath.Join(outside, "precious.txt"))
}

func TestAdversarial_SymlinkedRootNotFollowed(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	advPlant(t, outside, "precious.txt")
	link := filepath.Join(work, "pkg")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_ = advRun(t, evo.Config{}, evo.Tree{Path: link}.Remove)
	advPresent(t, filepath.Join(outside, "precious.txt"))
}

func TestAdversarial_FileRemoveOnSymlinkRemovesLinkOnly(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "target.txt")
	advPlant(t, outside, "target.txt")
	link := filepath.Join(work, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := advRun(t, evo.Config{}, evo.File{Path: link}.Remove); err != nil {
		t.Fatal(err)
	}
	advGone(t, link)
	advPresent(t, target)
}

func TestAdversarial_FileRemoveRefusesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	advPlant(t, dir, "keep.txt")
	err := advRun(t, evo.Config{}, evo.File{Path: dir}.Remove)
	if !errors.Is(err, evo.ErrFilePathTypeMismatch) {
		t.Fatalf("File.Remove on a directory = %v, want ErrFilePathTypeMismatch", err)
	}
	advPresent(t, filepath.Join(dir, "keep.txt"))
}

func TestAdversarial_TreeRemoveRefusesRegularFile(t *testing.T) {
	dir := t.TempDir()
	advPlant(t, dir, "f.txt")
	path := filepath.Join(dir, "f.txt")
	err := advRun(t, evo.Config{}, evo.Tree{Path: path}.Remove)
	if !errors.Is(err, evo.ErrTreePathTypeMismatch) {
		t.Fatalf("Tree.Remove on a file = %v, want ErrTreePathTypeMismatch", err)
	}
	advPresent(t, path)
}

// The Go module cache is 0555 all the way down; os.RemoveAll alone fails.
func TestAdversarial_ReadOnlyDirectoriesRemoved(t *testing.T) {
	root := filepath.Join(t.TempDir(), "mod", "example.com", "m@v1.0.0")
	advPlant(t, root, "go.mod", "sub/x.go", "sub/deeper/y.go")
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	})
	for _, dir := range slices.Backward(dirs) {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			_ = os.Chmod(d, 0o755)
		}
	})
	if err := advRun(t, evo.Config{}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatalf("Tree.Remove of a read-only tree: %v", err)
	}
	advGone(t, root)
}

// An empty Path must never resolve to the working directory.
func TestAdversarial_EmptyPathNeverMeansWorkingDirectory(t *testing.T) {
	work := t.TempDir()
	advPlant(t, work, "sentinel.txt")
	t.Chdir(work)
	for name, remove := range map[string]func(context.Context) error{
		"File": evo.File{}.Remove,
		"Tree": evo.Tree{}.Remove,
	} {
		if err := advRun(t, evo.Config{}, remove); !errors.Is(err, evo.ErrPathMissing) {
			t.Fatalf("%s{}.Remove = %v, want ErrPathMissing", name, err)
		}
		advPresent(t, filepath.Join(work, "sentinel.txt"))
	}
}

func TestAdversarial_NilContentWriteNeverDeletes(t *testing.T) {
	work := t.TempDir()
	advPlant(t, work, "f.txt", "tree/a.txt")
	file, tree := filepath.Join(work, "f.txt"), filepath.Join(work, "tree")
	if err := advRun(t, evo.Config{}, evo.File{Path: file}.Write); !errors.Is(err, evo.ErrContentMissing) {
		t.Fatalf("File.Write with nil Content = %v, want ErrContentMissing", err)
	}
	if err := advRun(t, evo.Config{}, evo.Tree{Path: tree}.Write); !errors.Is(err, evo.ErrContentMissing) {
		t.Fatalf("Tree.Write with nil Content = %v, want ErrContentMissing", err)
	}
	advPresent(t, file)
	advPresent(t, filepath.Join(tree, "a.txt"))
}

func TestAdversarial_RemoveMissingIsIdempotent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never")
	for range 2 {
		if err := advRun(t, evo.Config{}, evo.File{Path: missing}.Remove); err != nil {
			t.Fatalf("File.Remove of a missing path: %v", err)
		}
		if err := advRun(t, evo.Config{}, evo.Tree{Path: missing}.Remove); err != nil {
			t.Fatalf("Tree.Remove of a missing path: %v", err)
		}
	}
}

func TestAdversarial_CanceledRemoveLeavesTargetIntact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	advPlant(t, root, "a.js", "lib/b.js")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.Tree{Path: root}.Remove(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Remove = %v, want context.Canceled", err)
	}
	advPresent(t, filepath.Join(root, "a.js"))
	advPresent(t, filepath.Join(root, "lib", "b.js"))
}

// Structural: removal unlinks without opening files, so mode-000 files go.
func TestAdversarial_RemoveNeverReadsContents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	var rels []string
	for i := range 100 {
		rels = append(rels, fmt.Sprintf("d%d/f%d", i%5, i))
	}
	advPlant(t, root, rels...)
	for _, rel := range rels {
		if err := os.Chmod(filepath.Join(root, rel), 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := advRun(t, evo.Config{}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatalf("Tree.Remove opened file contents: %v", err)
	}
	advGone(t, root)
}

func BenchmarkTreeRemove_3000Files(b *testing.B) {
	work := b.TempDir()
	var rels []string
	for i := range 3000 {
		rels = append(rels, fmt.Sprintf("pkg%02d/f%04d.js", i%30, i))
	}
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for i := 0; b.Loop(); i++ {
			b.StopTimer()
			root := filepath.Join(work, fmt.Sprint(i))
			advPlant(b, root, rels...)
			b.StartTimer()
			if err := (evo.Tree{Path: root}).Remove(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

// A dangling symlink is a link, not an absence: File.Remove unlinks it.
func TestAdversarial_FileRemoveOnDanglingSymlinkRemovesLink(t *testing.T) {
	work := t.TempDir()
	link := filepath.Join(work, "dangling")
	if err := os.Symlink(filepath.Join(work, "nowhere"), link); err != nil {
		t.Fatal(err)
	}
	if err := advRun(t, evo.Config{}, evo.File{Path: link}.Remove); err != nil {
		t.Fatalf("File.Remove of a dangling symlink: %v", err)
	}
	advGone(t, link)
}

func TestAdversarial_CanceledFileRemoveLeavesFileIntact(t *testing.T) {
	work := t.TempDir()
	advPlant(t, work, "f.txt")
	path := filepath.Join(work, "f.txt")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.File{Path: path}.Remove(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled File.Remove = %v, want context.Canceled", err)
	}
	advPresent(t, path)
}

// A refused Remove must not disturb anything beside the target either.
func TestAdversarial_RefusedRemoveLeavesSiblingsAndMode(t *testing.T) {
	work := t.TempDir()
	advPlant(t, work, "d/inner.txt", "sibling.txt")
	_ = advRun(t, evo.Config{}, evo.File{Path: filepath.Join(work, "d")}.Remove)
	advPresent(t, filepath.Join(work, "d", "inner.txt"))
	advPresent(t, filepath.Join(work, "sibling.txt"))
}

// Writers and removers on one destination race from sibling Tasks. Evo owns
// the coordination: every call succeeds, and the settled state is either
// absent or exactly the writer's whole content, with nothing left beside it.
func TestAdversarial_RemoveRacingWritersLeavesAbsentOrWhole(t *testing.T) {
	const want = "whole-content-from-the-writer"
	for round := range 20 {
		work := t.TempDir()
		path := filepath.Join(work, "target.txt")
		advPlant(t, work, "target.txt")
		cfg := evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()}
		out := evo.Init(cfg)
		var tasks []interface{ Wait() error }
		for i := range 4 {
			name := fmt.Sprintf("racer-%d-%d", round, i)
			tasks = append(tasks, out.Task(name).Define(func(ctx context.Context) error {
				if i%2 == 0 {
					return evo.File{Path: path, Content: evo.Bytes(want)}.Write(ctx)
				}
				return evo.File{Path: path}.Remove(ctx)
			}))
		}
		for _, task := range tasks {
			if err := task.Wait(); err != nil {
				t.Fatalf("round %d: racing Task failed: %v", round, err)
			}
		}
		_ = out.Finish()
		_ = out.Close()
		if got, err := os.ReadFile(path); err == nil && string(got) != want {
			t.Fatalf("round %d: file holds %q, want absent or %q", round, got, want)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("round %d: %v", round, err)
		}
		entries, err := os.ReadDir(work)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != "target.txt" {
				t.Fatalf("round %d: stray entry %q beside the destination", round, e.Name())
			}
		}
	}
}

// Removers racing each other all succeed (idempotent under concurrency).
func TestAdversarial_ConcurrentRemovesAllSucceed(t *testing.T) {
	work := t.TempDir()
	root := filepath.Join(work, "pkg")
	advPlant(t, root, "a.js", "lib/b.js")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	var tasks []interface{ Wait() error }
	for i := range 6 {
		tasks = append(tasks, out.Task(fmt.Sprintf("remover-%d", i)).Define(evo.Tree{Path: root}.Remove))
	}
	for _, task := range tasks {
		if err := task.Wait(); err != nil {
			t.Fatalf("concurrent Tree.Remove: %v", err)
		}
	}
	_ = out.Finish()
	advGone(t, root)
}
