package checksum_test

// Adversarial hardening tests for File.Checksum and Tree.Checksum. Each test
// is named for the weakness it guards; sources are in
// docs/zys-1382/adversarial-research.md.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

const advLiveness = 10 * time.Second

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

func advPlant(tb testing.TB, root string, files map[string]string) {
	tb.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		tb.Fatal(err)
	}
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

// advTreeSum returns Tree.Checksum or the error it reported.
func advTreeSum(tb testing.TB, cfg evo.Config, root string, opts ...evo.ChecksumOption) (string, error) {
	tb.Helper()
	var sum string
	err := advRun(tb, cfg, func(ctx context.Context) error {
		var err error
		sum, err = evo.Tree{Path: root}.Checksum(ctx, opts...)
		return err
	})
	return sum, err
}

func advMustTreeSum(tb testing.TB, root string, opts ...evo.ChecksumOption) string {
	tb.Helper()
	sum, err := advTreeSum(tb, evo.Config{}, root, opts...)
	if err != nil {
		tb.Fatalf("Tree.Checksum(%s): %v", root, err)
	}
	return sum
}

// advWithin fails the test when fn does not return inside advLiveness.
func advWithin(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(advLiveness):
		t.Fatalf("%s did not return within %v", what, advLiveness)
	}
}

// countingFS counts content reads per path (ReadFile and the streaming Open,
// see contract-decisions.md Disputes 2026-10-01) on top of the real
// filesystem.
type countingFS struct {
	evo.FileFS
	mu    sync.Mutex
	reads map[string]int
}

func newCountingFS() *countingFS {
	return &countingFS{FileFS: testkit.NewFileFS(), reads: map[string]int{}}
}

func (c *countingFS) count(path string) {
	c.mu.Lock()
	c.reads[path]++
	c.mu.Unlock()
}

func (c *countingFS) ReadFile(path string) ([]byte, error) {
	c.count(path)
	return c.FileFS.ReadFile(path)
}

func (c *countingFS) Open(path string) (fs.File, error) {
	c.count(path)
	return os.Open(path)
}

// Naive concatenation of names and contents collides; framing must not.
func TestAdversarial_FramingDoesNotCollide(t *testing.T) {
	pairs := [][2]map[string]string{
		{{"a": "bc"}, {"ab": "c"}},
		{{"x/y": "z"}, {"x": "y/z"}}, // file under dir vs file whose content spells the path
		{{"a": "1", "b": "2"}, {"a": "1b2"}},
	}
	for i, pair := range pairs {
		left, right := filepath.Join(t.TempDir(), "l"), filepath.Join(t.TempDir(), "r")
		advPlant(t, left, pair[0])
		advPlant(t, right, pair[1])
		if advMustTreeSum(t, left) == advMustTreeSum(t, right) {
			t.Fatalf("pair %d collided: %v vs %v", i, pair[0], pair[1])
		}
	}
}

func TestAdversarial_EmptyDirectoryAndTypeAreStructure(t *testing.T) {
	bare, withDir, withFile := filepath.Join(t.TempDir(), "t"), filepath.Join(t.TempDir(), "t"), filepath.Join(t.TempDir(), "t")
	advPlant(t, bare, map[string]string{"a": "1"})
	advPlant(t, withDir, map[string]string{"a": "1"})
	advPlant(t, withFile, map[string]string{"a": "1", "x": ""})
	if err := os.Mkdir(filepath.Join(withDir, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	sums := map[string]string{"bare": advMustTreeSum(t, bare), "empty dir x": advMustTreeSum(t, withDir), "empty file x": advMustTreeSum(t, withFile)}
	seen := map[string]string{}
	for name, sum := range sums {
		if other, dup := seen[sum]; dup {
			t.Fatalf("%q and %q hash the same; structure and entry type must count", name, other)
		}
		seen[sum] = name
	}
}

func TestAdversarial_CreationOrderIrrelevant(t *testing.T) {
	names := []string{"z", "a", "a-b", "a.b", "a/b", "b", "é"} // no case-only pairs: APFS is case-insensitive
	forward, backward := filepath.Join(t.TempDir(), "t"), filepath.Join(t.TempDir(), "t")
	for i := range names {
		advPlant(t, forward, map[string]string{names[i] + "/f": names[i]})
		advPlant(t, backward, map[string]string{names[len(names)-1-i] + "/f": names[len(names)-1-i]})
	}
	if advMustTreeSum(t, forward) != advMustTreeSum(t, backward) {
		t.Fatal("creation order changed the tree checksum")
	}
}

func TestAdversarial_TimestampsIgnored(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1", "d/b": "2"})
	before := advMustTreeSum(t, root)
	past := time.Now().Add(-48 * time.Hour)
	for _, p := range []string{"a", "d/b", "d", "."} {
		if err := os.Chtimes(filepath.Join(root, p), past, past); err != nil {
			t.Fatal(err)
		}
	}
	if advMustTreeSum(t, root) != before {
		t.Fatal("atime/mtime changes altered the tree checksum")
	}
}

// Copies of a tree at different absolute locations must hash equal, or no
// cache keyed on the checksum ever hits.
func TestAdversarial_RootLocationIgnored(t *testing.T) {
	files := map[string]string{"package.json": "{}", "lib/i.js": "x"}
	a := filepath.Join(t.TempDir(), "one", "pkg")
	b := filepath.Join(t.TempDir(), "two", "deeper", "other-name")
	advPlant(t, a, files)
	advPlant(t, b, files)
	if advMustTreeSum(t, a) != advMustTreeSum(t, b) {
		t.Fatal("the tree's absolute location leaked into its checksum")
	}
}

func TestAdversarial_SymlinkNotFollowed(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1"})
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	before := advMustTreeSum(t, root)
	if err := os.WriteFile(outside, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if advMustTreeSum(t, root) != before {
		t.Fatal("changing a symlink's target content changed the tree checksum; the link was followed")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside+"-elsewhere", link); err != nil {
		t.Fatal(err)
	}
	if advMustTreeSum(t, root) == before {
		t.Fatal("retargeting a symlink did not change the tree checksum")
	}
}

func TestAdversarial_SymlinkLoopTerminates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"d/a": "1"})
	if err := os.Symlink("..", filepath.Join(root, "d", "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("self", filepath.Join(root, "self")); err != nil {
		t.Fatal(err)
	}
	advWithin(t, "Tree.Checksum over symlink loops", func() { _, _ = advTreeSum(t, evo.Config{}, root) })
}

func TestAdversarial_FIFODoesNotBlock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1"})
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	advWithin(t, "Tree.Checksum over a FIFO", func() { _, _ = advTreeSum(t, evo.Config{}, root) })
	advWithin(t, "File.Checksum of a FIFO", func() {
		_ = advRun(t, evo.Config{}, func(ctx context.Context) error {
			_, err := evo.File{Path: filepath.Join(root, "pipe")}.Checksum(ctx)
			return err
		})
	})
}

// An unreadable file must fail the checksum, never be skipped or treated as
// missing (which would report a false "unchanged").
func TestAdversarial_UnreadableFileIsError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-000 files")
	}
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1", "locked": "2"})
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	if _, err := advTreeSum(t, evo.Config{}, root); err == nil {
		t.Fatal("Tree.Checksum with an unreadable file succeeded")
	}
}

// The tree lives under a directory named .git; the exclusion must match the
// path inside the tree only.
func TestAdversarial_ExcludeMatchesInsidePathOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo", ".git", "worktree", "pkg")
	advPlant(t, root, map[string]string{"a.txt": "1", "sub/.git/HEAD": "ref: main"})
	exclude := evo.Exclude(`.*\/\.git\/.*`)
	before := advMustTreeSum(t, root, exclude)

	advPlant(t, root, map[string]string{"sub/.git/HEAD": "ref: other"})
	if advMustTreeSum(t, root, exclude) != before {
		t.Fatal("an excluded nested .git file changed the checksum")
	}
	advPlant(t, root, map[string]string{"a.txt": "2"})
	if advMustTreeSum(t, root, exclude) == before {
		t.Fatal("exclusion matched the root's own path and excluded the whole tree")
	}
}

func TestAdversarial_InvalidExcludeIsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1"})
	if _, err := advTreeSum(t, evo.Config{}, root, evo.Exclude(`([unclosed`)); err == nil {
		t.Fatal("an invalid exclusion regex was accepted (and would silently exclude nothing)")
	}
}

// Structural: an excluded subtree is never opened, so an unreadable file in
// it cannot fail the checksum.
func TestAdversarial_ExcludedSubtreeIsNotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-000 files")
	}
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1", "vendor/.git/objects/pack": "x"})
	locked := filepath.Join(root, "vendor", ".git", "objects", "pack")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	if _, err := advTreeSum(t, evo.Config{}, root, evo.Exclude(`.*\/\.git\/.*`)); err != nil {
		t.Fatalf("Tree.Checksum read an excluded file: %v", err)
	}
}

// Structural: one Tree.Checksum reads each regular file exactly once.
func TestAdversarial_TreeChecksumReadsEachFileOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	files := map[string]string{}
	for i := range 200 {
		files[fmt.Sprintf("d%02d/f%03d", i%10, i)] = fmt.Sprint(i)
	}
	advPlant(t, root, files)
	fsys := newCountingFS()
	if _, err := advTreeSum(t, evo.Config{FileFS: fsys}, root); err != nil {
		t.Fatal(err)
	}
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	for rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if n := fsys.reads[path]; n != 1 {
			t.Fatalf("%s read %d times through FileFS (ReadFile+Open), want exactly 1", rel, n)
		}
	}
	if len(fsys.reads) != len(files) {
		t.Fatalf("FileFS saw reads of %d paths, want %d", len(fsys.reads), len(files))
	}
}

// Structural: File.Checksum streams; it never buffers a large file whole.
func TestAdversarial_FileChecksumStreams(t *testing.T) {
	const size = 64 << 20
	path := filepath.Join(t.TempDir(), "big")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	var before, after runtime.MemStats
	err = advRun(t, evo.Config{}, func(ctx context.Context) error {
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err := evo.File{Path: path}.Checksum(ctx)
		runtime.ReadMemStats(&after)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > size/4 {
		t.Fatalf("File.Checksum of %d MiB allocated %d MiB; it buffers instead of streaming", size>>20, alloc>>20)
	}
}

// A cancelled context stops the walk and is reported as such, never as a
// digest of whatever was read before cancellation.
func TestAdversarial_CanceledContextIsHonoured(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	files := map[string]string{}
	for i := range 500 {
		files[fmt.Sprintf("d%02d/f%03d", i%20, i)] = fmt.Sprint(i)
	}
	advPlant(t, root, files)
	var treeSum, fileSum string
	var treeErr, fileErr error
	_ = advRun(t, evo.Config{}, func(ctx context.Context) error {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		treeSum, treeErr = evo.Tree{Path: root}.Checksum(canceled)
		fileSum, fileErr = evo.File{Path: filepath.Join(root, "d00", "f000")}.Checksum(canceled)
		return nil
	})
	if !errors.Is(treeErr, context.Canceled) || treeSum != "" {
		t.Errorf("Tree.Checksum with a cancelled context = (%q, %v), want context.Canceled and no digest", treeSum, treeErr)
	}
	if !errors.Is(fileErr, context.Canceled) || fileSum != "" {
		t.Errorf("File.Checksum with a cancelled context = (%q, %v), want context.Canceled and no digest", fileSum, fileErr)
	}
}

// Concurrent checksums of one tree agree (and are race-clean under -race).
func TestAdversarial_ConcurrentChecksumsAgree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "t")
	advPlant(t, root, map[string]string{"a": "1", "d/b": "2", "d/e/c": "3", ".git/HEAD": "ref"})
	want := advMustTreeSum(t, root, evo.Exclude(`.*\/\.git\/.*`))
	const workers = 8
	sums := make([]string, workers)
	errs := make([]error, workers)
	_ = advRun(t, evo.Config{}, func(ctx context.Context) error {
		var wg sync.WaitGroup
		for i := range workers {
			wg.Go(func() {
				sums[i], errs[i] = evo.Tree{Path: root}.Checksum(ctx, evo.Exclude(`.*\/\.git\/.*`))
			})
		}
		wg.Wait()
		return nil
	})
	for i := range workers {
		if errs[i] != nil || sums[i] != want {
			t.Fatalf("concurrent Tree.Checksum %d = (%q, %v), want %q", i, sums[i], errs[i], want)
		}
	}
}

// Options are values: one Exclude reused across trees and calls behaves the
// same every time (no state carried between calls).
func TestAdversarial_ExcludeOptionIsReusable(t *testing.T) {
	a, b := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	advPlant(t, a, map[string]string{"x": "1", ".git/HEAD": "one"})
	advPlant(t, b, map[string]string{"x": "1", "sub/.git/HEAD": "two"})
	exclude := evo.Exclude(`.*\/\.git\/.*`)
	first := advMustTreeSum(t, a, exclude)
	if advMustTreeSum(t, b, exclude) != first || advMustTreeSum(t, a, exclude) != first {
		t.Fatal("reusing one Exclude option across calls changed its effect")
	}
}

func BenchmarkTreeChecksum_3000SmallFiles(b *testing.B) {
	root := filepath.Join(b.TempDir(), "t")
	files := map[string]string{}
	for i := range 3000 {
		files[fmt.Sprintf("pkg%02d/lib/f%04d.js", i%30, i)] = fmt.Sprintf("module.exports = %d\n", i)
	}
	advPlant(b, root, files)
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for b.Loop() {
			if _, err := (evo.Tree{Path: root}).Checksum(ctx, evo.Exclude(`.*\/\.git\/.*`)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkFileChecksum_16MiB(b *testing.B) {
	const size = 16 << 20
	path := filepath.Join(b.TempDir(), "big")
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(size)
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for b.Loop() {
			if _, err := (evo.File{Path: path}).Checksum(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}
