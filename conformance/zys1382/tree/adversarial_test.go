package tree_test

// Adversarial hardening tests for evo.Tree. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// advEntry is one tar member. Typeflag zero means a regular file.
type advEntry struct {
	name, body, link string
	typeflag         byte
}

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

// advTarGz writes entries as a tar.gz at path and returns it as a File.
func advTarGz(tb testing.TB, path string, entries ...advEntry) evo.File {
	tb.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Linkname: e.link, Mode: 0o644, Typeflag: e.typeflag}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(e.body))
		}
		if hdr.Typeflag == tar.TypeDir {
			hdr.Mode = 0o755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tb.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				tb.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		tb.Fatal(err)
	}
	return evo.File{Path: path}
}

// advFlat returns n regular-file entries named prefix-NNNN.
func advFlat(prefix string, n int) []advEntry {
	entries := make([]advEntry, n)
	for i := range entries {
		entries[i] = advEntry{name: fmt.Sprintf("%s-%04d", prefix, i), body: prefix}
	}
	return entries
}

// advSnapshot maps each regular file under root (slash path) to content.
func advSnapshot(tb testing.TB, root string) map[string]string {
	tb.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		got[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	return got
}

func advNames(tb testing.TB, dir string) []string {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func advWriteTree(tb testing.TB, dest string, archive evo.File) error {
	tb.Helper()
	return advRun(tb, evo.Config{}, evo.Tree{Path: dest, Content: evo.Extract{File: archive}}.Write)
}

func TestAdversarial_FailedWritePreservesPreviousTree(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	good := advTarGz(t, filepath.Join(work, "good.tgz"), advEntry{name: "a.txt", body: "v1"}, advEntry{name: "lib/b.txt", body: "v1"})
	if err := advWriteTree(t, dest, good); err != nil {
		t.Fatalf("seed Write: %v", err)
	}
	want := advSnapshot(t, dest)
	bad := advTarGz(t, filepath.Join(work, "bad.tgz"),
		advEntry{name: "a.txt", body: "v2"}, advEntry{name: "lib/b.txt", body: "v2"}, advEntry{name: "../escape.txt", body: "x"})
	if err := advWriteTree(t, dest, bad); err == nil {
		t.Fatal("Write of an archive with a traversal entry succeeded")
	}
	if got := advSnapshot(t, dest); !maps.Equal(got, want) {
		t.Fatalf("failed Write left a mixed tree: %v, want %v", got, want)
	}
}

func TestAdversarial_ReplaceLeavesNoResidue(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	v1 := advTarGz(t, filepath.Join(work, "v1.tgz"), advEntry{name: "old.js", body: "1"}, advEntry{name: "keep.js", body: "1"})
	v2 := advTarGz(t, filepath.Join(work, "v2.tgz"), advEntry{name: "keep.js", body: "2"}, advEntry{name: "new.js", body: "2"})
	for _, archive := range []evo.File{v1, v2} {
		if err := advWriteTree(t, dest, archive); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{"keep.js": "2", "new.js": "2"}
	if got := advSnapshot(t, dest); !maps.Equal(got, want) {
		t.Fatalf("replacement merged trees: %v", got)
	}
}

func TestAdversarial_DestinationSymlinkNotWrittenThrough(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	dest := filepath.Join(work, "pkg")
	if err := os.Symlink(outside, dest); err != nil {
		t.Fatal(err)
	}
	archive := advTarGz(t, filepath.Join(work, "a.tgz"), advEntry{name: "pwn.txt", body: "x"})
	_ = advWriteTree(t, dest, archive)
	if names := advNames(t, outside); len(names) != 0 {
		t.Fatalf("Write went through the destination symlink: %v", names)
	}
}

func TestAdversarial_ConcurrentWritersPublishWholeTrees(t *testing.T) {
	const writers = 8
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	archives := [2]evo.File{
		advTarGz(t, filepath.Join(work, "a.tgz"), advFlat("a", 200)...),
		advTarGz(t, filepath.Join(work, "b.tgz"), advFlat("b", 200)...),
	}
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("writers")
	for i := range writers {
		tree := evo.Tree{Path: dest, Content: evo.Extract{File: archives[i%2]}}
		group.Task(fmt.Sprintf("w%d", i)).Define(tree.Write)
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("writers: %v", err)
	}
	_ = out.Finish()
	advRequireOneWholeTree(t, dest, 200)
}

// Structural: an observer polling the destination during repeated
// replacements only ever sees a whole tree, or nothing.
func TestAdversarial_ObserverNeverSeesPartialTree(t *testing.T) {
	const files = 300
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	archives := [2]evo.File{
		advTarGz(t, filepath.Join(work, "a.tgz"), advFlat("a", files)...),
		advTarGz(t, filepath.Join(work, "b.tgz"), advFlat("b", files)...),
	}
	var done atomic.Bool
	var partial atomic.Value
	var wg sync.WaitGroup
	wg.Go(func() {
		for !done.Load() {
			entries, err := os.ReadDir(dest)
			if err != nil {
				continue // absent between publications is acceptable
			}
			if msg := advWholeTreeProblem(entries, files); msg != "" {
				partial.Store(msg)
				return
			}
		}
	})
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		for i := range 10 {
			if err := (evo.Tree{Path: dest, Content: evo.Extract{File: archives[i%2]}}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	done.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if msg := partial.Load(); msg != nil {
		t.Fatalf("observer saw a partial tree: %v", msg)
	}
}

func TestAdversarial_FailedWriteLeavesNoStagingInParent(t *testing.T) {
	work := t.TempDir()
	parent := filepath.Join(work, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := advTarGz(t, filepath.Join(work, "bad.tgz"), append(advFlat("a", 50), advEntry{name: "/abs.txt", body: "x"})...)
	if err := advWriteTree(t, filepath.Join(parent, "pkg"), bad); err == nil {
		t.Fatal("Write of an archive with an absolute entry succeeded")
	}
	if names := advNames(t, parent); len(names) != 0 {
		t.Fatalf("failed Write left staging entries beside the destination: %v", names)
	}
}

func TestAdversarial_VerifyDetectsAddedFile(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: advTarGz(t, filepath.Join(work, "a.tgz"), advEntry{name: "a.txt", body: "a"})}}
	if err := advRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "planted.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := advRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify with an added file = %v, want ErrVerifyMismatch", err)
	}
}

func TestAdversarial_CanceledWritePublishesNothing(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	archive := advTarGz(t, filepath.Join(work, "a.tgz"), advFlat("a", 500)...)
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.Tree{Path: dest, Content: evo.Extract{File: archive}}.Write(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Write = %v, want context.Canceled", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("canceled Write published the destination")
	}
}

// A symlink inside the old tree is replaced as a link, never followed: the
// directory it pointed at keeps its content.
func TestAdversarial_ReplaceDoesNotFollowSymlinksInOldTree(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "precious.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(work, "pkg")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Fatal(err)
	}
	archive := advTarGz(t, filepath.Join(work, "a.tgz"), advEntry{name: "a.txt", body: "a"})
	if err := advWriteTree(t, dest, archive); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(outside, "precious.txt")); err != nil || string(body) != "keep" {
		t.Fatalf("replacing a tree followed a symlink and damaged its target: %q, %v", body, err)
	}
}

// Remove deletes the link, not what it points at.
func TestAdversarial_RemoveDoesNotFollowSymlinksInTree(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "precious.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(work, "pkg")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Fatal(err)
	}
	if err := advRun(t, evo.Config{}, evo.Tree{Path: dest}.Remove); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("tree still present after Remove: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(outside, "precious.txt")); err != nil || string(body) != "keep" {
		t.Fatalf("Remove followed a symlink and deleted its target: %q, %v", body, err)
	}
}

// Remove of a symlink to a directory must never delete the directory's content.
func TestAdversarial_RemoveOfASymlinkedRootNeverDeletesTheTarget(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "precious.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(work, "pkg")
	if err := os.Symlink(outside, dest); err != nil {
		t.Fatal(err)
	}
	_ = advRun(t, evo.Config{}, evo.Tree{Path: dest}.Remove)
	if body, err := os.ReadFile(filepath.Join(outside, "precious.txt")); err != nil || string(body) != "keep" {
		t.Fatalf("Remove through a symlinked root deleted the target's content: %q, %v", body, err)
	}
}

// A different link target is a different tree; a link is never dereferenced
// for Equal.
func TestAdversarial_EqualComparesLinkTextNotLinkTargets(t *testing.T) {
	work := t.TempDir()
	same := t.TempDir()
	if err := os.WriteFile(filepath.Join(same, "f"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "f"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	for dir, target := range map[string]string{a: same, b: other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
	}
	var equal bool
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		equal, err = evo.Tree{Path: a}.Equal(ctx, evo.Tree{Path: b})
		return err
	})
	if err != nil {
		t.Fatalf("Equal: %v", err)
	}
	if equal {
		t.Fatalf("Equal followed symlinks: links with different targets compared equal")
	}
}

func TestAdversarial_EmptyArchiveWriteFailsAndPreservesPreviousTree(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	good := advTarGz(t, filepath.Join(work, "good.tgz"), advEntry{name: "a.txt", body: "v1"})
	if err := advWriteTree(t, dest, good); err != nil {
		t.Fatalf("seed Write: %v", err)
	}
	empty := advTarGz(t, filepath.Join(work, "empty.tgz"))
	if err := advWriteTree(t, dest, empty); !errors.Is(err, evo.ErrExtractMalformed) {
		t.Fatalf("Write of an empty archive = %v, want ErrExtractMalformed", err)
	}
	if got := advSnapshot(t, dest); !maps.Equal(got, map[string]string{"a.txt": "v1"}) {
		t.Fatalf("a failed Write damaged the previous tree: %v", got)
	}
}

func TestAdversarial_MissingArchivePreservesPreviousTree(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	good := advTarGz(t, filepath.Join(work, "good.tgz"), advEntry{name: "a.txt", body: "v1"})
	if err := advWriteTree(t, dest, good); err != nil {
		t.Fatalf("seed Write: %v", err)
	}
	err := advWriteTree(t, dest, evo.File{Path: filepath.Join(work, "absent.tgz")})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write from a missing archive = %v, want fs.ErrNotExist", err)
	}
	if got := advSnapshot(t, dest); !maps.Equal(got, map[string]string{"a.txt": "v1"}) {
		t.Fatalf("a failed Write damaged the previous tree: %v", got)
	}
}

func TestAdversarial_CanceledWritePreservesPreviousTree(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	v1 := advTarGz(t, filepath.Join(work, "v1.tgz"), advEntry{name: "a.txt", body: "v1"})
	if err := advWriteTree(t, dest, v1); err != nil {
		t.Fatalf("seed Write: %v", err)
	}
	v2 := advTarGz(t, filepath.Join(work, "v2.tgz"), advFlat("b", 300)...)
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.Tree{Path: dest, Content: evo.Extract{File: v2}}.Write(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Write = %v, want context.Canceled", err)
	}
	if got := advSnapshot(t, dest); !maps.Equal(got, map[string]string{"a.txt": "v1"}) {
		t.Fatalf("canceled Write damaged the previous tree: %v", got)
	}
	if names := advNames(t, work); len(names) != 3 {
		t.Fatalf("canceled Write left staging entries in the parent: %v", names)
	}
}

func TestAdversarial_ReadDoesNotFollowSymlinksOrListSpecialFiles(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(work, "pkg")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	var files []evo.File
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		files, err = evo.Tree{Path: root}.Read(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, root+string(filepath.Separator)) || strings.Contains(f.Path, "secret") {
			t.Fatalf("Read escaped the tree through a symlink: %s", f.Path)
		}
	}
	if len(files) != 1 {
		t.Fatalf("Read returned %d files, want only the regular file a.txt", len(files))
	}
}

func TestAdversarial_ReadIsContentLazy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(root, "locked.txt")
	if err := os.WriteFile(unreadable, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	var files []evo.File
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		files, err = evo.Tree{Path: root}.Read(ctx)
		return err
	})
	if err != nil || len(files) != 1 || files[0].Content != nil {
		t.Fatalf("Read of an unreadable file = %v, %v; enumeration must not open files", files, err)
	}
}

func advWholeTreeProblem(entries []fs.DirEntry, want int) string {
	if len(entries) != want {
		return fmt.Sprintf("%d entries, want %d", len(entries), want)
	}
	prefix, _, _ := strings.Cut(entries[0].Name(), "-")
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix+"-") {
			return fmt.Sprintf("mixed entries %q and %q", entries[0].Name(), e.Name())
		}
	}
	return ""
}

func advRequireOneWholeTree(t *testing.T, dest string, want int) {
	t.Helper()
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if msg := advWholeTreeProblem(entries, want); msg != "" {
		t.Fatalf("destination is not one writer's whole tree: %s", msg)
	}
}

func BenchmarkTreeWrite_2000Files(b *testing.B) {
	work := b.TempDir()
	archives := [2]evo.File{
		advTarGz(b, filepath.Join(work, "a.tgz"), advFlat("a", 2000)...),
		advTarGz(b, filepath.Join(work, "b.tgz"), advFlat("b", 2000)...),
	}
	dest := filepath.Join(work, "pkg")
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for i := 0; b.Loop(); i++ {
			if err := (evo.Tree{Path: dest, Content: evo.Extract{File: archives[i%2]}}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}
