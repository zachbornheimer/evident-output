// Package checksum_test is the ZYS-1382 execution contract for
// File.Checksum and Tree.Checksum: each struct owns its method, both route
// through one internal engine (observed here through Config.FileFS), Basis
// fingerprints use the same engine, and tree exclusions are path-based.
// Names and open-syntax choices: docs/zys-1382/contract-decisions.md.
package checksum_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
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
func fileSum(t *testing.T, path string) string {
	t.Helper()
	return fileSumIn(t, evo.Config{}, path)
}

func fileSumIn(t *testing.T, cfg evo.Config, path string) string {
	t.Helper()
	var sum string
	err := contractRun(t, cfg, func(ctx context.Context) error {
		var err error
		sum, err = evo.File{Path: path}.Checksum(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("File.Checksum(%s): %v", path, err)
	}
	return sum
}

func treeSum(t *testing.T, path string, opts ...evo.ChecksumOption) string {
	t.Helper()
	return treeSumIn(t, evo.Config{}, path, opts...)
}

func treeSumIn(t *testing.T, cfg evo.Config, path string, opts ...evo.ChecksumOption) string {
	t.Helper()
	var sum string
	err := contractRun(t, cfg, func(ctx context.Context) error {
		var err error
		sum, err = evo.Tree{Path: path}.Checksum(ctx, opts...)
		return err
	})
	if err != nil {
		t.Fatalf("Tree.Checksum(%s): %v", path, err)
	}
	return sum
}

// sha256Hex is the File.Checksum of data.
func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// requireDigestForm fails unless sum is lowercase hex of sha256.Size bytes,
// the one digest form File.Checksum and Tree.Checksum share.
func requireDigestForm(t *testing.T, what, sum string) {
	t.Helper()
	raw, err := hex.DecodeString(sum)
	if err != nil || len(raw) != sha256.Size || sum != strings.ToLower(sum) {
		t.Fatalf("%s = %q, want %d lowercase hex characters", what, sum, sha256.Size*2)
	}
}

// forgingFS is the observable seam into the one checksum engine: content
// reads through Config.FileFS see forged bytes for chosen absolute paths,
// while the disk keeps its own. A digest that reflects the forgery proves the
// read went through Evo's content path; one that reflects the disk proves a
// second, private path exists.
type forgingFS struct {
	evo.FileFS
	forged map[string]string
}

func newForgingFS(forged map[string]string) forgingFS {
	return forgingFS{FileFS: testkit.NewFileFS(), forged: forged}
}

func (f forgingFS) ReadFile(path string) ([]byte, error) {
	if body, ok := f.forged[path]; ok {
		return []byte(body), nil
	}
	return f.FileFS.ReadFile(path)
}

// Open is the streaming read the engine needs to hash large files without
// buffering them (contract-decisions.md, Disputes 2026-10-01). Forged paths
// stream their forged bytes.
func (f forgingFS) Open(path string) (fs.File, error) {
	if body, ok := f.forged[path]; ok {
		return fstest.MapFS{"forged": {Data: []byte(body), Mode: 0o644}}.Open("forged")
	}
	return os.Open(path)
}

var baseTree = map[string]string{
	"package.json":   `{"name":"pkg"}`,
	"src/a.go":       "package a\n",
	"src/deep/b.go":  "package b\n",
	"docs/readme.md": "# pkg\n",
}

func TestFileChecksumIsTheSHA256OfItsBytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"text", []byte("hello\n")},
		{"empty", nil},
		{"binary", []byte{0x00, 0xff, 0x10, 0x80}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.bin")
			if err := os.WriteFile(path, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			want := sha256.Sum256(tc.data)
			if got := fileSum(t, path); got != hex.EncodeToString(want[:]) {
				t.Fatalf("File.Checksum = %q, want lowercase hex SHA-256 %q", got, hex.EncodeToString(want[:]))
			}
		})
	}
}

func TestFileChecksumIgnoresNameModeAndTimes(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "renamed.bin")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("same bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(b, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(b, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if fileSum(t, a) != fileSum(t, b) {
		t.Fatalf("File.Checksum differs for identical bytes under a different name, mode, and mtime")
	}
}

func TestFileChecksumOfAMissingFileIsNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.File{Path: path}.Checksum(ctx)
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("File.Checksum of a missing file = %v, want fs.ErrNotExist", err)
	}
}

func TestTreeChecksumOfAMissingTreeIsNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.Tree{Path: path}.Checksum(ctx)
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Tree.Checksum of a missing tree = %v, want fs.ErrNotExist", err)
	}
}

func TestTreeChecksumIsDeterministic(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "nested", "elsewhere", "b")
	plant(t, a, baseTree)
	// b is created in a different order, at a different root, at another time.
	for _, rel := range []string{"src/deep/b.go", "docs/readme.md", "src/a.go", "package.json"} {
		plant(t, b, map[string]string{rel: baseTree[rel]})
		path := filepath.Join(b, filepath.FromSlash(rel))
		if err := os.Chtimes(path, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
	}
	first := treeSum(t, a)
	if again := treeSum(t, a); again != first {
		t.Fatalf("Tree.Checksum is not stable across calls: %q then %q", first, again)
	}
	if other := treeSum(t, b); other != first {
		t.Fatalf("identical trees at different roots checksum differently: %q vs %q", first, other)
	}
	requireDigestForm(t, "Tree.Checksum", first)
	requireDigestForm(t, "File.Checksum", fileSum(t, filepath.Join(a, "package.json")))
}

func TestTreeChecksumCoversStructureAndContents(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{"a file's bytes change", func(t *testing.T, root string) {
			plant(t, root, map[string]string{"src/a.go": "package changed\n"})
		}},
		{"a file is added", func(t *testing.T, root string) {
			plant(t, root, map[string]string{"src/new.go": "package a\n"})
		}},
		{"a file is removed", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "docs", "readme.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a file is renamed with identical bytes", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "src", "a.go"), filepath.Join(root, "src", "z.go")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a file moves to another directory with identical bytes", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "src", "a.go"), filepath.Join(root, "docs", "a.go")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "tree")
			plant(t, root, baseTree)
			before := treeSum(t, root)
			tc.mutate(t, root)
			if after := treeSum(t, root); after == before {
				t.Fatalf("Tree.Checksum did not change")
			}
		})
	}
}

// A tree's digest depends on a leaf only through that leaf's File.Checksum:
// swapping a leaf for another file with the same File.Checksum keeps the
// tree digest, and any leaf whose File.Checksum changes moves it.
func TestTreeChecksumLeafIsDerivedFromFileChecksum(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	plant(t, a, baseTree)
	plant(t, b, baseTree)
	leafA, leafB := filepath.Join(a, "src", "a.go"), filepath.Join(b, "src", "a.go")

	// Same bytes, different mode and mtime: File.Checksum is equal.
	if err := os.Chmod(leafB, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(leafB, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if fileSum(t, leafA) != fileSum(t, leafB) {
		t.Fatalf("leaf File.Checksums differ for identical bytes")
	}
	if treeSum(t, a) != treeSum(t, b) {
		t.Fatalf("trees whose leaves have equal File.Checksums checksum differently")
	}

	if err := os.WriteFile(leafB, []byte("package other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fileSum(t, leafA) == fileSum(t, leafB) {
		t.Fatalf("leaf File.Checksum did not change with its bytes")
	}
	if treeSum(t, a) == treeSum(t, b) {
		t.Fatalf("tree digest did not move when one leaf's File.Checksum changed")
	}
}

// Exclude matches an unanchored regex against "/" + the slash-separated path
// inside the tree, with a trailing "/" on directories, so the ticket's
// `.*\/\.git\/.*` removes a .git subtree at the root and at any depth.
func TestTreeChecksumExcludeIsPathBased(t *testing.T) {
	cases := []struct {
		name    string
		extra   map[string]string
		exclude []string
	}{
		{"a .git subtree at the root", map[string]string{
			".git/HEAD": "ref: refs/heads/main\n", ".git/objects/ab/cdef": "blob",
		}, []string{`.*\/\.git\/.*`}},
		{"a .git subtree at depth", map[string]string{
			"src/deep/.git/HEAD": "ref: refs/heads/main\n",
		}, []string{`.*\/\.git\/.*`}},
		{"a single file by its full in-tree path", map[string]string{
			"src/generated.go": "package a\n",
		}, []string{`^/src/generated\.go$`}},
		{"files by extension", map[string]string{
			"src/a.log": "noise", "debug.log": "noise",
		}, []string{`\.log$`}},
		{"several exclusions combine", map[string]string{
			".git/HEAD": "ref", "debug.log": "noise",
		}, []string{`.*\/\.git\/.*`, `\.log$`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			clean, noisy := filepath.Join(work, "clean"), filepath.Join(work, "noisy")
			plant(t, clean, baseTree)
			plant(t, noisy, baseTree)
			plant(t, noisy, tc.extra)
			opts := make([]evo.ChecksumOption, len(tc.exclude))
			for i, pattern := range tc.exclude {
				opts[i] = evo.Exclude(pattern)
			}
			want := treeSum(t, clean)
			if got := treeSum(t, noisy); got == want {
				t.Fatalf("the extra entries did not change the unexcluded checksum")
			}
			if got := treeSum(t, noisy, opts...); got != want {
				t.Fatalf("excluded checksum = %q, want the clean tree's %q", got, want)
			}
		})
	}
}

func TestTreeChecksumExcludeMatchesInsideTheTreeNotTheRootPath(t *testing.T) {
	// The tree's own location contains ".git"; only in-tree paths are matched.
	root := filepath.Join(t.TempDir(), ".git", "tree")
	plant(t, root, baseTree)
	if treeSum(t, root, evo.Exclude(`.*\/\.git\/.*`)) != treeSum(t, root) {
		t.Fatalf("Exclude matched the tree's own root path")
	}
}

// One engine: File.Checksum and every leaf of Tree.Checksum read content
// through the same seam, so a forged leaf moves both exactly as if the disk
// held the forged bytes.
func TestFileAndTreeChecksumRouteThroughOneEngine(t *testing.T) {
	work := t.TempDir()
	disk, twin := filepath.Join(work, "disk"), filepath.Join(work, "twin")
	plant(t, disk, baseTree)
	plant(t, twin, baseTree)
	const forged = "package forged\n"
	plant(t, twin, map[string]string{"src/a.go": forged})
	leaf := filepath.Join(disk, "src", "a.go")
	cfg := evo.Config{FileFS: newForgingFS(map[string]string{leaf: forged})}

	if got := fileSumIn(t, cfg, leaf); got != sha256Hex(forged) {
		t.Fatalf("File.Checksum = %q, want the digest of the bytes read through Config.FileFS %q; File.Checksum bypasses the engine", got, sha256Hex(forged))
	}
	got := treeSumIn(t, cfg, disk)
	if got == treeSum(t, disk) {
		t.Fatalf("Tree.Checksum ignored the leaf bytes read through Config.FileFS; Tree.Checksum bypasses the engine")
	}
	if want := treeSum(t, twin); got != want {
		t.Fatalf("Tree.Checksum with a forged leaf = %q, want the checksum of a tree holding those bytes %q", got, want)
	}
}

// Basis fingerprints route through the same engine: freshness follows the
// bytes the engine reads, never a private read of the disk.
func TestBasisRoutesThroughTheChecksumEngine(t *testing.T) {
	cases := []struct {
		name  string
		basis func(work string) func(*evo.TaskHandle) *evo.TaskHandle
	}{
		{"File", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.File{Path: filepath.Join(work, "src", "a.go")})
			}
		}},
		{"Tree", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.Tree{Path: work})
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work, state := filepath.Join(t.TempDir(), "work"), t.TempDir()
			plant(t, work, baseTree)
			leaf := filepath.Join(work, "src", "a.go")
			basis := tc.basis(work)
			forge := func(body string) evo.FileFS { return newForgingFS(map[string]string{leaf: body}) }

			if !basisRan(t, state, forge("v1"), basis) {
				t.Fatalf("first run: the Task never ran")
			}
			plant(t, work, map[string]string{"src/a.go": "edited on disk only\n"})
			if basisRan(t, state, forge("v1"), basis) {
				t.Fatalf("Basis reran on a disk edit the engine never saw; Basis reads content outside the checksum engine")
			}
			if !basisRan(t, state, forge("v2"), basis) {
				t.Fatalf("Basis stayed current although the engine read new bytes")
			}
		})
	}
}

// basisRan runs one Task named "install" with basis on a fresh Output sharing
// stateDir, reading content through fsys, and reports whether it ran.
func basisRan(t *testing.T, stateDir string, fsys evo.FileFS, basis func(*evo.TaskHandle) *evo.TaskHandle) bool {
	t.Helper()
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard,
		StateDir: stateDir, FileFS: fsys,
	})
	var ran atomic.Bool
	task := basis(out.Task("install")).Define(func(context.Context) error {
		ran.Store(true)
		return nil
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	return ran.Load()
}

func TestTreeChecksumOfAnEmptyTree(t *testing.T) {
	work := t.TempDir()
	empty, holdsEmptyFile := filepath.Join(work, "empty"), filepath.Join(work, "one")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	plant(t, holdsEmptyFile, map[string]string{"x": ""})
	sum := treeSum(t, empty)
	requireDigestForm(t, "Tree.Checksum of an empty tree", sum)
	if sum == treeSum(t, holdsEmptyFile) {
		t.Fatalf("an empty tree and a tree holding one empty file checksum the same")
	}
}

func TestTreeChecksumExcludingEverythingIsTheEmptyTree(t *testing.T) {
	work := t.TempDir()
	empty, full := filepath.Join(work, "empty"), filepath.Join(work, "full")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	plant(t, full, baseTree)
	if treeSum(t, full, evo.Exclude(`.`)) != treeSum(t, empty) {
		t.Fatalf("a tree with every in-tree path excluded does not checksum as the empty tree")
	}
}

// Directories carry a trailing "/": excluding one excludes its subtree, and a
// pattern without the slash does not match the directory.
func TestTreeChecksumExcludeDirectoryCarriesTrailingSlash(t *testing.T) {
	work := t.TempDir()
	full, withoutSrc := filepath.Join(work, "full"), filepath.Join(work, "without-src")
	plant(t, full, baseTree)
	for rel, body := range baseTree {
		if !strings.HasPrefix(rel, "src/") {
			plant(t, withoutSrc, map[string]string{rel: body})
		}
	}
	if got, want := treeSum(t, full, evo.Exclude(`^/src/$`)), treeSum(t, withoutSrc); got != want {
		t.Fatalf("Exclude(`^/src/$`) = %q, want the checksum of the tree without src %q", got, want)
	}
	if treeSum(t, full, evo.Exclude(`^/src$`)) != treeSum(t, full) {
		t.Fatalf("Exclude(`^/src$`) matched the directory src; directories carry a trailing slash")
	}
}

// The tree's own root is never matched, even by a pattern that names it.
func TestTreeChecksumExcludeNeverMatchesTheRootItself(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree")
	plant(t, root, baseTree)
	if treeSum(t, root, evo.Exclude(`^/$`)) != treeSum(t, root) {
		t.Fatalf("Exclude(`^/$`) changed the checksum; the tree's root location must never be matched")
	}
}

// File is one regular file and Tree one directory; each refuses the other's
// cardinality instead of inventing a digest.
func TestChecksumRefusesTheOtherCardinality(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tree")
	plant(t, dir, baseTree)
	file := filepath.Join(dir, "package.json")
	var fileErr, treeErr error
	var fileDigest, treeDigest string
	_ = contractRun(t, evo.Config{}, func(ctx context.Context) error {
		fileDigest, fileErr = evo.File{Path: dir}.Checksum(ctx)
		treeDigest, treeErr = evo.Tree{Path: file}.Checksum(ctx)
		return nil
	})
	if fileErr == nil || fileDigest != "" {
		t.Errorf("File.Checksum of a directory = (%q, %v), want an error and no digest", fileDigest, fileErr)
	}
	if treeErr == nil || treeDigest != "" {
		t.Errorf("Tree.Checksum of a regular file = (%q, %v), want an error and no digest", treeDigest, treeErr)
	}
}

func TestChecksumOfAnEmptyPathIsPathMissing(t *testing.T) {
	var fileErr, treeErr error
	_ = contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, fileErr = evo.File{}.Checksum(ctx)
		_, treeErr = evo.Tree{}.Checksum(ctx)
		return nil
	})
	if !errors.Is(fileErr, evo.ErrPathMissing) {
		t.Errorf("File{}.Checksum = %v, want ErrPathMissing", fileErr)
	}
	if !errors.Is(treeErr, evo.ErrPathMissing) {
		t.Errorf("Tree{}.Checksum = %v, want ErrPathMissing", treeErr)
	}
}

// Checksum observes; declared Content is not what it reports.
func TestChecksumReportsTheDiskNotTheDeclaredContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sum string
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		sum, err = evo.File{Path: path, Content: evo.Bytes("declared")}.Checksum(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum != sha256Hex("on disk") {
		t.Fatalf("File.Checksum = %q, want the digest of the bytes on disk %q", sum, sha256Hex("on disk"))
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "on disk" {
		t.Fatalf("File.Checksum changed the file: %q, %v", got, err)
	}
}
