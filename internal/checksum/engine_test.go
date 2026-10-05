package checksum

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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

func mustTree(t *testing.T, e Engine, root string, patterns ...string) Digest {
	t.Helper()
	exclude, err := CompileExclusion(patterns...)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.Tree(context.Background(), root, exclude)
	if err != nil {
		t.Fatalf("Tree(%s): %v", root, err)
	}
	return d
}

// recorder collects Observe events.
type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) observe(e Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *recorder) count(op Op) map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := map[string]int{}
	for _, e := range r.events {
		if e.Op == op {
			n[e.Path]++
		}
	}
	return n
}

func TestFileDigestIsSHA256OfBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	plant(t, filepath.Dir(path), map[string]string{"f": "hello\n"})
	got, err := Engine{}.File(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if want := Digest(sha256.Sum256([]byte("hello\n"))); got != want {
		t.Fatalf("File = %s, want %s", got, want)
	}
	if len(got.String()) != sha256.Size*2 || got.String() != strings.ToLower(got.String()) {
		t.Fatalf("String() = %q, want 64 lowercase hex characters", got)
	}
}

func TestFileRefusesSymlinkAndDirectory(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"f": "x"})
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "f"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := (Engine{}).File(context.Background(), link); !errors.Is(err, ErrSymlink) {
		t.Errorf("File(symlink) = %v, want ErrSymlink", err)
	}
	if _, err := (Engine{}).File(context.Background(), dir); !errors.Is(err, ErrNotRegular) {
		t.Errorf("File(dir) = %v, want ErrNotRegular", err)
	}
	if _, err := (Engine{}).Tree(context.Background(), filepath.Join(dir, "f"), Exclusion{}); !errors.Is(err, ErrNotDirectory) {
		t.Errorf("Tree(file) = %v, want ErrNotDirectory", err)
	}
}

// Every leaf goes through the one Strategy, exactly once, and an excluded
// subtree is never listed or read.
func TestTreeRoutesEveryLeafThroughTheEngineOnce(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"a": "1", "d/b": "2", "d/e/c": "3", ".git/HEAD": "ref", "x/.git/objects/o": "blob"}
	plant(t, root, files)
	rec := &recorder{}
	mustTree(t, Engine{Observe: rec.observe}, root, `.*\/\.git\/.*`)
	leaves := rec.count(OpLeaf)
	for _, rel := range []string{"a", "d/b", "d/e/c"} {
		if n := leaves[filepath.Join(root, filepath.FromSlash(rel))]; n != 1 {
			t.Errorf("%s digested %d times, want 1", rel, n)
		}
	}
	if len(leaves) != 3 {
		t.Errorf("digested %d leaves, want 3: %v", len(leaves), leaves)
	}
	for path := range rec.count(OpList) {
		if strings.Contains(path, ".git") {
			t.Errorf("excluded directory %s was listed", path)
		}
	}
}

// fixedStrategy proves the Strategy seam: whatever it returns is the leaf,
// for File and for every tree leaf alike.
type fixedStrategy struct{ d Digest }

func (f fixedStrategy) Leaf(context.Context, Source, string, fs.FileInfo) (Digest, error) {
	return f.d, nil
}

func TestStrategyIsTheOneLeafFunction(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	plant(t, a, map[string]string{"x": "one", "d/y": "two"})
	plant(t, b, map[string]string{"x": "three", "d/y": "four"})
	fixed := Engine{Strategy: fixedStrategy{d: Digest{1}}}
	if got, _ := fixed.File(context.Background(), filepath.Join(a, "x")); got != (Digest{1}) {
		t.Fatalf("File ignored the Strategy: %s", got)
	}
	if mustTree(t, fixed, a) != mustTree(t, fixed, b) {
		t.Fatal("trees whose leaves the Strategy digests identically differ")
	}
	if mustTree(t, Engine{}, a) == mustTree(t, Engine{}, b) {
		t.Fatal("Content strategy did not distinguish different bytes")
	}
}

func TestTreeIsDeterministicAcrossParallelism(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{}
	for i := range 300 {
		files[fmt.Sprintf("d%02d/f%03d", i%13, i)] = fmt.Sprint(i)
	}
	plant(t, root, files)
	want := mustTree(t, Engine{Parallelism: 1}, root)
	for _, p := range []int{2, 7, 64} {
		if got := mustTree(t, Engine{Parallelism: p}, root); got != want {
			t.Fatalf("Parallelism %d = %s, want %s", p, got, want)
		}
	}
}

func TestTreeFramingAndStructure(t *testing.T) {
	cases := [][2]map[string]string{
		{{"a": "bc"}, {"ab": "c"}},
		{{"x/y": "z"}, {"x": "y/z"}},
		{{"a": "1", "b": "2"}, {"a": "1b2"}},
	}
	for i, pair := range cases {
		l, r := filepath.Join(t.TempDir(), "l"), filepath.Join(t.TempDir(), "r")
		plant(t, l, pair[0])
		plant(t, r, pair[1])
		if mustTree(t, Engine{}, l) == mustTree(t, Engine{}, r) {
			t.Errorf("pair %d collided", i)
		}
	}
	empty, holdsEmptyDir := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(holdsEmptyDir, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if mustTree(t, Engine{}, empty) == mustTree(t, Engine{}, holdsEmptyDir) {
		t.Error("an empty directory entry did not count")
	}
}

func TestTreeExclusionRules(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"src/a": "1", "keep": "2"})
	full := mustTree(t, Engine{}, root)
	if mustTree(t, Engine{}, root, `^/src$`) != full {
		t.Error("a directory matched without its trailing slash")
	}
	if mustTree(t, Engine{}, root, `^/$`) != full {
		t.Error("the root itself was matched")
	}
	if mustTree(t, Engine{}, root, `^/src/$`) == full {
		t.Error("a directory pattern with its trailing slash matched nothing")
	}
	if _, err := CompileExclusion(`([`); !errors.Is(err, ErrInvalidExclude) {
		t.Errorf("CompileExclusion(invalid) = %v, want ErrInvalidExclude", err)
	}
}

// failingSource fails one leaf's open with errBoom.
type failingSource struct {
	OS
	path string
}

var errBoom = errors.New("boom")

func (f failingSource) Open(path string) (io.ReadCloser, error) {
	if path == f.path {
		return nil, errBoom
	}
	return f.OS.Open(path)
}

func TestTreeLeafFailureSurfacesItself(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{}
	for i := range 100 {
		files[fmt.Sprintf("f%03d", i)] = fmt.Sprint(i)
	}
	plant(t, root, files)
	e := Engine{Source: failingSource{path: filepath.Join(root, "f042")}, Parallelism: 4}
	_, err := e.Tree(context.Background(), root, Exclusion{})
	if !errors.Is(err, errBoom) {
		t.Fatalf("Tree with a failing leaf = %v, want errBoom", err)
	}
}

func TestCancelledContextReturnsNoDigest(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"a": "1"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Engine{}).Tree(ctx, root, Exclusion{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Tree(cancelled) = %v, want context.Canceled", err)
	}
	if _, err := (Engine{}).File(ctx, filepath.Join(root, "a")); !errors.Is(err, context.Canceled) {
		t.Errorf("File(cancelled) = %v, want context.Canceled", err)
	}
}

func TestSymlinkIsDigestedAsItsTarget(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"a": "1"})
	if err := os.Symlink("a", filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	before := mustTree(t, Engine{}, root)
	plant(t, root, map[string]string{"a": "2"})
	afterContent := mustTree(t, Engine{}, root)
	if err := os.Remove(filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("b", filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	if before == afterContent || mustTree(t, Engine{}, root) == afterContent {
		t.Fatal("symlink target text or leaf content did not count")
	}
}
