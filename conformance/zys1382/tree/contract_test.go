// Package tree_test is the ZYS-1382 execution contract for evo.Tree: one
// directory tree reconciled with the same vocabulary as evo.File. Names and
// open-syntax choices: docs/zys-1382/contract-decisions.md.
package tree_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// Sealed content producer a Tree accepts.
var _ evo.TreeContent = evo.Extract{}

// reconciled is the vocabulary File and Tree share with identical
// signatures. Both satisfy it as plain values, so literals call it directly.
type reconciled interface {
	Write(ctx context.Context) error
	Verify(ctx context.Context) error
	Remove(ctx context.Context) error
}

var (
	_ reconciled = evo.File{}
	_ reconciled = evo.Tree{}
)

// Exact method shapes, as value-receiver method expressions: the compiler
// rejects a pointer receiver, a missing method, or a different signature.
var (
	_ func(evo.Tree, context.Context) ([]evo.File, error)                            = evo.Tree.Read
	_ func(evo.Tree, context.Context) error                                          = evo.Tree.Write
	_ func(evo.Tree, context.Context) error                                          = evo.Tree.Verify
	_ func(evo.Tree, context.Context, evo.Tree, ...evo.ChecksumOption) (bool, error) = evo.Tree.Equal
	_ func(evo.Tree, context.Context) error                                          = evo.Tree.Remove
	_ func(evo.Tree, context.Context, ...evo.ChecksumOption) (string, error)         = evo.Tree.Checksum
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

// archive writes a tar.gz holding files to dir/name and returns it as a File.
func archive(t *testing.T, dir, name string, files map[string]string) evo.File {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for rel := range files {
		names = append(names, rel)
	}
	slices.Sort(names)
	for _, rel := range names {
		hdr := &tar.Header{Name: rel, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(files[rel]))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(files[rel])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return evo.File{Path: path}
}

// onDisk returns every regular file under root as slash path -> content.
func onDisk(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		body, readErr := os.ReadFile(path)
		got[filepath.ToSlash(rel)] = string(body)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func equalFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

var packageFiles = map[string]string{
	"package.json":     `{"name":"pkg"}`,
	"lib/index.js":     "module.exports = 1\n",
	"lib/deep/util.js": "exports.util = true\n",
}

func TestTreeIsAPlainStructLiteral(t *testing.T) {
	archive := evo.File{Path: "pkg.tgz"}
	tree := evo.Tree{Path: "node_modules/pkg", Content: evo.Extract{File: archive, Root: "package"}}
	if tree.Path != "node_modules/pkg" || tree.Content == nil {
		t.Fatalf("Tree literal fields did not round-trip: %+v", tree)
	}
}

func TestTreeWriteEstablishesDesiredContent(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "out", "pkg")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packageFiles) {
		t.Fatalf("tree on disk = %v, want %v", got, packageFiles)
	}
}

func TestTreeWriteReplacesADifferingTreeWholesale(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "out", "pkg")
	plant(t, dest, map[string]string{"package.json": "stale", "stale/only-in-old.js": "stale"})
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packageFiles) {
		t.Fatalf("tree on disk = %v, want exactly %v", got, packageFiles)
	}
	if _, err := os.Lstat(filepath.Join(dest, "stale")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a directory absent from the desired tree survived Write")
	}
}

func TestTreeWriteAlreadySatisfiedDoesNotRepublish(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "out", "pkg")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	marker := filepath.Join(dest, "package.json")
	before, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	after, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("an already-satisfied Write republished the tree")
	}
}

func TestTreeWritePublishesAtomicallyLeavingNoSiblings(t *testing.T) {
	work := t.TempDir()
	parent := filepath.Join(work, "out")
	dest := filepath.Join(parent, "pkg")
	plant(t, dest, map[string]string{"package.json": "stale"})
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "pkg" {
		t.Fatalf("Write left siblings beside the destination: %v", entries)
	}
}

func TestTreeWriteIsVerifiedAfterCommit(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := tree.Write(ctx); err != nil {
			return err
		}
		return tree.Verify(ctx)
	})
	if err != nil {
		t.Fatalf("Write then Verify: %v", err)
	}
}

func TestTreeVerifyComparesDiskToDesiredState(t *testing.T) {
	cases := []struct {
		name   string
		tamper func(t *testing.T, dest string)
	}{
		{"a file's content changed", func(t *testing.T, dest string) {
			plant(t, dest, map[string]string{"lib/index.js": "tampered"})
		}},
		{"a stray file appeared", func(t *testing.T, dest string) {
			plant(t, dest, map[string]string{"lib/stray.js": "stray"})
		}},
		{"a file disappeared", func(t *testing.T, dest string) {
			if err := os.Remove(filepath.Join(dest, "package.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"the tree disappeared", func(t *testing.T, dest string) {
			if err := os.RemoveAll(dest); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "pkg")
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
			if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
				t.Fatalf("Write: %v", err)
			}
			tc.tamper(t, dest)
			if err := contractRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
				t.Fatalf("Verify = %v, want ErrVerifyMismatch", err)
			}
		})
	}
}

func TestTreeReadReturnsItsFilesSortedAndContentLazy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	plant(t, root, packageFiles)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	var (
		files []evo.File
		body  []byte
	)
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		if files, err = (evo.Tree{Path: root}).Read(ctx); err != nil {
			return err
		}
		body, err = files[0].Read(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := []string{
		filepath.Join(root, "lib", "deep", "util.js"),
		filepath.Join(root, "lib", "index.js"),
		filepath.Join(root, "package.json"),
	}
	got := make([]string, len(files))
	for i, f := range files {
		got[i] = f.Path
		if f.Content != nil {
			t.Fatalf("Read loaded content for %s; want a content-lazy File", f.Path)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Read paths = %v, want %v", got, want)
	}
	if string(body) != packageFiles["lib/deep/util.js"] {
		t.Fatalf("a File returned by Tree.Read read %q", body)
	}
}

func TestTreeReadOfAMissingTreeIsNotExist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.Tree{Path: root}.Read(ctx)
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Read of a missing tree = %v, want fs.ErrNotExist", err)
	}
}

func TestTreeEqualComparesObservedStructureAndContent(t *testing.T) {
	cases := []struct {
		name  string
		other map[string]string
		want  bool
	}{
		{"identical trees", packageFiles, true},
		{"one file's content differs", map[string]string{
			"package.json": `{"name":"other"}`, "lib/index.js": "module.exports = 1\n", "lib/deep/util.js": "exports.util = true\n",
		}, false},
		{"one file is named differently", map[string]string{
			"package.json": `{"name":"pkg"}`, "lib/main.js": "module.exports = 1\n", "lib/deep/util.js": "exports.util = true\n",
		}, false},
		{"one file is missing", map[string]string{
			"package.json": `{"name":"pkg"}`, "lib/index.js": "module.exports = 1\n",
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			a, b := filepath.Join(work, "a"), filepath.Join(work, "elsewhere", "b")
			plant(t, a, packageFiles)
			plant(t, b, tc.other)
			var got bool
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				var err error
				got, err = evo.Tree{Path: a}.Equal(ctx, evo.Tree{Path: b})
				return err
			})
			if err != nil {
				t.Fatalf("Equal: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTreeEqualHonorsExclusions(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	plant(t, a, packageFiles)
	plant(t, b, packageFiles)
	plant(t, b, map[string]string{".git/HEAD": "ref: refs/heads/main\n"})
	var plain, excluded bool
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		if plain, err = (evo.Tree{Path: a}).Equal(ctx, evo.Tree{Path: b}); err != nil {
			return err
		}
		excluded, err = evo.Tree{Path: a}.Equal(ctx, evo.Tree{Path: b}, evo.Exclude(`.*\/\.git\/.*`))
		return err
	})
	if err != nil {
		t.Fatalf("Equal: %v", err)
	}
	if plain || !excluded {
		t.Fatalf("Equal = %v without Exclude and %v with it, want false and true", plain, excluded)
	}
}

func TestTreeWriteRequiresAPath(t *testing.T) {
	work := t.TempDir()
	tree := evo.Tree{Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrPathMissing) {
		t.Fatalf("Write with empty Path = %v, want ErrPathMissing", err)
	}
}

func TestTreeWriteUnderDryRunMutatesNothing(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{DryRun: true}, tree.Write); err != nil {
		t.Fatalf("dry-run Write: %v", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry-run Write created the tree")
	}
}

func TestTreeWriteWithNilContentIsContentMissingAndLeavesThePathUntouched(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "pkg")
	plant(t, dest, packageFiles)
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: dest}.Write); !errors.Is(err, evo.ErrContentMissing) {
		t.Fatalf("Write with nil Content = %v, want ErrContentMissing", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packageFiles) {
		t.Fatalf("a nil-Content Write changed the tree: %v", got)
	}
}

func TestTreeNilContentNeverMeansAbsence(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "pkg")
	plant(t, dest, packageFiles)
	_ = contractRun(t, evo.Config{}, evo.Tree{Path: dest}.Write)
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("nil Content removed the tree: %v", err)
	}
}

func TestTreeWriteStripsExtractRoot(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "node_modules", "pkg")
	prefixed := map[string]string{}
	for rel, body := range packageFiles {
		prefixed["package/"+rel] = body
	}
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", prefixed), Root: "package"}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := tree.Write(ctx); err != nil {
			return err
		}
		return tree.Verify(ctx)
	})
	if err != nil {
		t.Fatalf("Write/Verify with Root: %v", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packageFiles) {
		t.Fatalf("tree on disk = %v, want the archive's files without the %q prefix", got, "package")
	}
}

func TestTreeWriteOutsideATaskIsNoTaskContext(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := tree.Write(context.Background()); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Write outside a Task = %v, want ErrNoTaskContext", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write outside a Task mutated the filesystem")
	}
}

func TestTreeReadAndVerifyAndRemoveRequireAPath(t *testing.T) {
	ops := map[string]func(ctx context.Context) error{
		"Read":   func(ctx context.Context) error { _, err := evo.Tree{}.Read(ctx); return err },
		"Verify": evo.Tree{}.Verify,
		"Remove": evo.Tree{}.Remove,
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			if err := contractRun(t, evo.Config{}, op); !errors.Is(err, evo.ErrPathMissing) {
				t.Fatalf("%s with empty Path = %v, want ErrPathMissing", name, err)
			}
		})
	}
}

func TestTreeRemoveDeletesTheWholeTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	plant(t, root, packageFiles)
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("tree still present after Remove: %v", err)
	}
}

func TestTreeRemoveOfAnAbsentPathIsSuccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatalf("Remove of an absent tree = %v, want nil", err)
	}
}

func TestTreeRemoveOfARegularFileIsATypeMismatchAndDeletesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: path}.Remove); !errors.Is(err, evo.ErrTreePathTypeMismatch) {
		t.Fatalf("Tree.Remove on a regular file = %v, want ErrTreePathTypeMismatch", err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "keep" {
		t.Fatalf("type-mismatched Remove touched the file: %q, %v", body, err)
	}
}

func TestTreeRemoveUnderDryRunMutatesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	plant(t, root, packageFiles)
	if err := contractRun(t, evo.Config{DryRun: true}, evo.Tree{Path: root}.Remove); err != nil {
		t.Fatalf("dry-run Remove: %v", err)
	}
	if got := onDisk(t, root); !equalFiles(got, packageFiles) {
		t.Fatalf("dry-run Remove changed the tree: %v", got)
	}
}

func TestTreeRemoveOutsideATaskIsNoTaskContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pkg")
	plant(t, root, packageFiles)
	if err := (evo.Tree{Path: root}).Remove(context.Background()); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Remove outside a Task = %v, want ErrNoTaskContext", err)
	}
	if got := onDisk(t, root); !equalFiles(got, packageFiles) {
		t.Fatalf("Remove outside a Task mutated the tree")
	}
}

func TestTreeEqualTreatsEmptyDirectoriesAsStructure(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	plant(t, a, packageFiles)
	plant(t, b, packageFiles)
	if err := os.MkdirAll(filepath.Join(b, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	var got bool
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		got, err = evo.Tree{Path: a}.Equal(ctx, evo.Tree{Path: b})
		return err
	})
	if err != nil || got {
		t.Fatalf("Equal with an extra empty directory = %v, %v; want false, nil", got, err)
	}
}

func TestTreeEqualIsSymmetricAndReflexive(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	plant(t, a, packageFiles)
	plant(t, b, map[string]string{"package.json": "other"})
	var aa, ab, ba bool
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		if aa, err = (evo.Tree{Path: a}).Equal(ctx, evo.Tree{Path: a}); err != nil {
			return err
		}
		if ab, err = (evo.Tree{Path: a}).Equal(ctx, evo.Tree{Path: b}); err != nil {
			return err
		}
		ba, err = evo.Tree{Path: b}.Equal(ctx, evo.Tree{Path: a})
		return err
	})
	if err != nil || !aa || ab || ba {
		t.Fatalf("Equal(a,a)=%v Equal(a,b)=%v Equal(b,a)=%v err=%v; want true false false nil", aa, ab, ba, err)
	}
}

// Equal and Checksum are two views of one identity: trees are Equal exactly
// when their checksums agree, under the same exclusions.
func TestTreeEqualAgreesWithChecksum(t *testing.T) {
	work := t.TempDir()
	a, b := filepath.Join(work, "a"), filepath.Join(work, "b")
	plant(t, a, packageFiles)
	plant(t, b, packageFiles)
	exclude := evo.Exclude(`.*\/\.git\/.*`)
	plant(t, b, map[string]string{".git/HEAD": "x"})
	var sumA, sumB, sumAEx, sumBEx string
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		if sumA, err = (evo.Tree{Path: a}).Checksum(ctx); err != nil {
			return err
		}
		if sumB, err = (evo.Tree{Path: b}).Checksum(ctx); err != nil {
			return err
		}
		if sumAEx, err = (evo.Tree{Path: a}).Checksum(ctx, exclude); err != nil {
			return err
		}
		sumBEx, err = evo.Tree{Path: b}.Checksum(ctx, exclude)
		return err
	})
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if sumA == sumB || sumAEx != sumBEx {
		t.Fatalf("checksums disagree with Equal: plain a=%s b=%s, excluded a=%s b=%s", sumA, sumB, sumAEx, sumBEx)
	}
}

func TestTreeVerifyAfterWriteMatchesDeclaredContentNotMerelyExistence(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	plant(t, dest, map[string]string{"unrelated.txt": "x"})
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	if err := contractRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify of a different existing tree = %v, want ErrVerifyMismatch", err)
	}
}

func TestTreeConcurrentWritesFromSiblingTasksAllSucceed(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, work, "pkg.tgz", packageFiles)}}
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("writers")
	for i := range 4 {
		group.Task(fmt.Sprintf("w%d", i)).Define(tree.Write)
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("sibling Writes: %v", err)
	}
	_ = out.Finish()
	if got := onDisk(t, dest); !equalFiles(got, packageFiles) {
		t.Fatalf("tree after concurrent Writes = %v", got)
	}
}
