package checksum_test

// Routing and exclusion-edge tests for the one checksum engine. The engine's
// only injectable seam is Config.FileFS (content reads: ReadFile, or the
// streaming Open proposed in contract-decisions.md Disputes). Every test here
// proves a property through that seam rather than by comparing outputs of two
// paths that could each hold a private hash.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// errSeam is the sentinel failingFS returns; seeing it in a Checksum error
// proves the read reached the engine's seam and the error was not swallowed.
var errSeam = errors.New("checksum seam: injected read failure")

// failingFS fails every content read of one absolute path with errSeam.
type failingFS struct {
	evo.FileFS
	path string
}

func (f failingFS) ReadFile(path string) ([]byte, error) {
	if path == f.path {
		return nil, errSeam
	}
	return f.FileFS.ReadFile(path)
}

func (f failingFS) Open(path string) (fs.File, error) {
	if path == f.path {
		return nil, errSeam
	}
	return os.Open(path)
}

// A read failure at the seam reaches both callers wrapped, never a digest:
// File.Checksum and Tree.Checksum read leaves through the same engine.
func TestChecksumReadFailureAtTheSeamSurfacesFromFileAndTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree")
	plant(t, root, baseTree)
	leaf := filepath.Join(root, "src", "deep", "b.go")
	cfg := evo.Config{FileFS: failingFS{FileFS: testkit.NewFileFS(), path: leaf}}
	var fileSum, treeSum string
	var fileErr, treeErr error
	_ = contractRun(t, cfg, func(ctx context.Context) error {
		fileSum, fileErr = evo.File{Path: leaf}.Checksum(ctx)
		treeSum, treeErr = evo.Tree{Path: root}.Checksum(ctx)
		return nil
	})
	if !errors.Is(fileErr, errSeam) || fileSum != "" {
		t.Errorf("File.Checksum with a failing seam = (%q, %v), want errSeam and no digest", fileSum, fileErr)
	}
	if !errors.Is(treeErr, errSeam) || treeSum != "" {
		t.Errorf("Tree.Checksum with a failing leaf = (%q, %v), want errSeam and no digest", treeSum, treeErr)
	}
}

// An excluded leaf is never read, so a seam failure on it cannot fail the
// tree: exclusion happens before the engine touches content.
func TestChecksumSeamIsNotReachedForAnExcludedLeaf(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree")
	plant(t, root, baseTree)
	plant(t, root, map[string]string{".git/HEAD": "ref"})
	leaf := filepath.Join(root, ".git", "HEAD")
	cfg := evo.Config{FileFS: failingFS{FileFS: testkit.NewFileFS(), path: leaf}}
	if _, err := treeSumErr(t, cfg, root, evo.Exclude(`.*\/\.git\/.*`)); err != nil {
		t.Fatalf("Tree.Checksum read an excluded leaf through the seam: %v", err)
	}
}

// File.Checksum reads its one path through the seam exactly once.
func TestFileChecksumReadsItsPathOnceThroughTheSeam(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := newCountingFS()
	fileSumIn(t, evo.Config{FileFS: fsys}, path)
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	if n := fsys.reads[path]; n != 1 || len(fsys.reads) != 1 {
		t.Fatalf("File.Checksum content reads through FileFS = %v, want exactly one read of %s", fsys.reads, path)
	}
}

// File.Equal compares bytes through the same engine: a forged read makes two
// files equal exactly when their engine digests are equal.
func TestFileEqualRoutesThroughTheChecksumEngine(t *testing.T) {
	dir := t.TempDir()
	disk, twin := filepath.Join(dir, "disk"), filepath.Join(dir, "twin")
	const forged = "forged bytes"
	for path, body := range map[string]string{disk: "disk bytes", twin: forged} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := evo.Config{FileFS: newForgingFS(map[string]string{disk: forged})}
	var equal bool
	err := contractRun(t, cfg, func(ctx context.Context) error {
		var err error
		equal, err = evo.File{Path: disk}.Equal(ctx, evo.File{Path: twin})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !equal {
		t.Fatalf("File.Equal read %s outside the engine: the seam serves it the twin's bytes, so they must be equal", disk)
	}
}

// Tree.Equal is Checksum equality under the same options, read through the
// same seam.
func TestTreeEqualAgreesWithTreeChecksum(t *testing.T) {
	work := t.TempDir()
	clean, noisy, forgedTwin := filepath.Join(work, "clean"), filepath.Join(work, "noisy"), filepath.Join(work, "twin")
	plant(t, clean, baseTree)
	plant(t, noisy, baseTree)
	plant(t, noisy, map[string]string{"sub/.git/HEAD": "ref"})
	plant(t, forgedTwin, baseTree)
	plant(t, forgedTwin, map[string]string{"src/a.go": "package forged\n"})
	exclude := evo.Exclude(`.*\/\.git\/.*`)
	cases := []struct {
		name  string
		cfg   evo.Config
		left  string
		right string
		opts  []evo.ChecksumOption
		want  bool
	}{
		{"extra .git subtree, no options", evo.Config{}, clean, noisy, nil, false},
		{"extra .git subtree, excluded", evo.Config{}, clean, noisy, []evo.ChecksumOption{exclude}, true},
		{"leaf forged through the seam", evo.Config{FileFS: newForgingFS(map[string]string{
			filepath.Join(clean, "src", "a.go"): "package forged\n",
		})}, clean, forgedTwin, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var equal bool
			err := contractRun(t, tc.cfg, func(ctx context.Context) error {
				var err error
				equal, err = evo.Tree{Path: tc.left}.Equal(ctx, evo.Tree{Path: tc.right}, tc.opts...)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if equal != tc.want {
				t.Fatalf("Tree.Equal = %v, want %v", equal, tc.want)
			}
			sumsEqual := treeSumIn(t, tc.cfg, tc.left, tc.opts...) == treeSumIn(t, tc.cfg, tc.right, tc.opts...)
			if sumsEqual != equal {
				t.Fatalf("Tree.Equal = %v but Tree.Checksum equality = %v; both must come from one engine", equal, sumsEqual)
			}
		})
	}
}

// In a linked git worktree `.git` is a regular file, path "/.git" with no
// trailing slash, so `.*\/\.git\/.*` does not exclude it (decided rule:
// only directories carry the trailing "/").
func TestTreeChecksumExcludeLeavesAGitFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	plant(t, root, baseTree)
	plant(t, root, map[string]string{".git": "gitdir: /elsewhere/one\n"})
	exclude := evo.Exclude(`.*\/\.git\/.*`)
	before := treeSum(t, root, exclude)
	plant(t, root, map[string]string{".git": "gitdir: /elsewhere/two\n"})
	if treeSum(t, root, exclude) == before {
		t.Fatalf("Exclude(`.*\\/\\.git\\/.*`) excluded a regular file named .git; only directories carry a trailing slash")
	}
}

// Paths begin with "/": a pattern anchored without it matches nothing.
func TestTreeChecksumExcludePathsStartWithASlash(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree")
	plant(t, root, baseTree)
	if treeSum(t, root, evo.Exclude(`^src/`)) != treeSum(t, root) {
		t.Fatalf("Exclude(`^src/`) matched; in-tree paths are matched as \"/src/...\"")
	}
	if treeSum(t, root, evo.Exclude(`^/src/`)) == treeSum(t, root) {
		t.Fatalf("Exclude(`^/src/`) matched nothing; in-tree paths start with \"/\"")
	}
}

// Excluding every file in a directory leaves the directory: the result is the
// tree holding that directory empty, not the tree without it.
func TestTreeChecksumExcludedFileLeavesItsDirectory(t *testing.T) {
	work := t.TempDir()
	full, emptyDocs := filepath.Join(work, "full"), filepath.Join(work, "empty-docs")
	plant(t, full, baseTree)
	for rel, body := range baseTree {
		if rel != "docs/readme.md" {
			plant(t, emptyDocs, map[string]string{rel: body})
		}
	}
	if err := os.Mkdir(filepath.Join(emptyDocs, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := treeSum(t, full, evo.Exclude(`^/docs/readme\.md$`)), treeSum(t, emptyDocs); got != want {
		t.Fatalf("excluding docs/readme.md = %q, want the tree with an empty docs/ %q", got, want)
	}
}

// treeSumErr returns Tree.Checksum or its error without failing the test.
func treeSumErr(t *testing.T, cfg evo.Config, path string, opts ...evo.ChecksumOption) (string, error) {
	t.Helper()
	var sum string
	err := contractRun(t, cfg, func(ctx context.Context) error {
		var err error
		sum, err = evo.Tree{Path: path}.Checksum(ctx, opts...)
		return err
	})
	return sum, err
}
