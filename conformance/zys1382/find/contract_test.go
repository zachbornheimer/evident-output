// Package find_test is the ZYS-1382 execution contract for evo.Find:
// recursive, content-lazy discovery that returns File values and never
// locks the searched tree. Names and open-syntax choices:
// docs/zys-1382/contract-decisions.md.
package find_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

var workspace = map[string]string{
	"package.json":                        `{"name":"root"}`,
	"pnpm-lock.yaml":                      "lockfileVersion: 9\n",
	"packages/a/package.json":             `{"name":"a"}`,
	"packages/a/src/index.ts":             "export {}\n",
	"packages/b/package.json":             `{"name":"b"}`,
	"packages/b/nested/deep/package.json": `{"name":"deep"}`,
	"node_modules/dep/package.json":       `{"name":"dep"}`,
	"README.md":                           "# root\n",
}

func paths(root string, files []evo.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		rel, _ := filepath.Rel(root, f.Path)
		out[i] = filepath.ToSlash(rel)
	}
	return out
}

func find(t *testing.T, root string, names ...string) []evo.File {
	t.Helper()
	var files []evo.File
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		files, err = evo.Find(ctx, root, names...)
		return err
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	return files
}

func TestFindReturnsFilesByBaseNameRecursively(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	cases := []struct {
		name  string
		names []string
		want  []string
	}{
		{"one name at every depth", []string{"package.json"}, []string{
			"node_modules/dep/package.json", "package.json", "packages/a/package.json",
			"packages/b/nested/deep/package.json", "packages/b/package.json",
		}},
		{"several names", []string{"package.json", "pnpm-lock.yaml"}, []string{
			"node_modules/dep/package.json", "package.json", "packages/a/package.json",
			"packages/b/nested/deep/package.json", "packages/b/package.json", "pnpm-lock.yaml",
		}},
		{"a name that matches nothing", []string{"yarn.lock"}, []string{}},
		{"a name is a base name, not a path", []string{"a/package.json"}, []string{}},
		{"a leading separator matches nothing", []string{"/package.json"}, []string{}},
		{"a dot-relative name matches nothing", []string{"./package.json"}, []string{}},
		{"matching is case-sensitive", []string{"PACKAGE.JSON"}, []string{}},
		{"glob characters are literal", []string{"*.json", "package.*", "pack?ge.json"}, []string{}},
		{"a parent-relative name matches nothing", []string{"a/../package.json", "..", "."}, []string{}},
		{"an empty name matches nothing", []string{""}, []string{}},
		{"an empty name does not hide the others", []string{"", "pnpm-lock.yaml"}, []string{"pnpm-lock.yaml"}},
		{"a repeated name returns each file once", []string{"pnpm-lock.yaml", "pnpm-lock.yaml"}, []string{"pnpm-lock.yaml"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := paths(root, find(t, root, tc.names...))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Find = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFindResultsAreSortedAbsoluteAndRegularFilesOnly(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	// A directory that shares a searched name must not be returned.
	if err := os.MkdirAll(filepath.Join(root, "dirs", "package.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := find(t, root, "package.json")
	for _, f := range files {
		if !filepath.IsAbs(f.Path) {
			t.Fatalf("Find returned a relative Path %q", f.Path)
		}
		info, err := os.Lstat(f.Path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("Find returned a non-regular entry %q", f.Path)
		}
	}
	got := paths(root, files)
	if !slices.IsSorted(got) {
		t.Fatalf("Find results are not sorted: %v", got)
	}
	if slices.Contains(got, "dirs/package.json") {
		t.Fatalf("Find returned a directory")
	}
}

func TestFindIsContentLazy(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	files := find(t, root, "package.json")
	for _, f := range files {
		if f.Content != nil {
			t.Fatalf("Find loaded Content for %s", f.Path)
		}
	}
	idx := slices.IndexFunc(files, func(f evo.File) bool { return f.Path == filepath.Join(root, "package.json") })
	if idx < 0 {
		t.Fatalf("root package.json missing from %v", paths(root, files))
	}
	var (
		body []byte
		sum  string
	)
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		if body, err = files[idx].Read(ctx); err != nil {
			return err
		}
		sum, err = files[idx].Checksum(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("Read/Checksum on a found File: %v", err)
	}
	if string(body) != workspace["package.json"] {
		t.Fatalf("found File read %q", body)
	}
	raw := sha256.Sum256([]byte(workspace["package.json"]))
	if sum != hex.EncodeToString(raw[:]) {
		t.Fatalf("found File Checksum = %s, want the SHA-256 of its bytes", sum)
	}
}

// countingFS counts content reads through the FileFS facade and otherwise
// delegates to the real filesystem.
type countingFS struct{ reads atomic.Int64 }

func (c *countingFS) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (c *countingFS) ReadFile(path string) ([]byte, error) {
	c.reads.Add(1)
	return os.ReadFile(path)
}
func (c *countingFS) WriteAtomic(path string, b []byte, mode fs.FileMode) error {
	return os.WriteFile(path, b, mode)
}
func (c *countingFS) Chmod(path string, mode fs.FileMode) error { return os.Chmod(path, mode) }

// "Content-lazy (no reads)": a whole search, matches included, performs
// zero content reads through the FileFS facade.
func TestFindPerformsZeroContentReads(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	counter := &countingFS{}
	var files []evo.File
	err := contractRun(t, evo.Config{FileFS: counter}, func(ctx context.Context) error {
		var err error
		files, err = evo.Find(ctx, root, "package.json", "pnpm-lock.yaml")
		return err
	})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(files) != 6 {
		t.Fatalf("Find returned %d files, want 6", len(files))
	}
	if n := counter.reads.Load(); n != 0 {
		t.Fatalf("Find read file content %d times, want 0", n)
	}
}

// Found Files are plain File values: Path set, nothing else declared.
func TestFindReturnsFileValuesAtTheSearchedPath(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	for _, f := range find(t, root, "package.json") {
		if f.Content != nil {
			t.Fatalf("%s carries Content", f.Path)
		}
		rel, err := filepath.Rel(root, f.Path)
		if err != nil || f.Path != filepath.Join(root, rel) || strings.HasPrefix(rel, "..") {
			t.Fatalf("Path %q is not under the searched root %q", f.Path, root)
		}
	}
}

// A relative root yields absolute Paths, and a trailing separator or an
// unclean root does not change the answer.
func TestFindNormalizesTheRoot(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	want := paths(root, find(t, root, "package.json"))
	t.Run("trailing separator", func(t *testing.T) {
		got := paths(root, find(t, root+string(filepath.Separator), "package.json"))
		if !slices.Equal(got, want) {
			t.Fatalf("Find = %v, want %v", got, want)
		}
	})
	t.Run("unclean", func(t *testing.T) {
		got := paths(root, find(t, filepath.Join(root, "packages", ".."), "package.json"))
		if !slices.Equal(got, want) {
			t.Fatalf("Find = %v, want %v", got, want)
		}
	})
	t.Run("relative", func(t *testing.T) {
		t.Chdir(root)
		files := find(t, ".", "package.json")
		for _, f := range files {
			if !filepath.IsAbs(f.Path) {
				t.Fatalf("relative root produced relative Path %q", f.Path)
			}
		}
		if len(files) != len(want) {
			t.Fatalf("relative root found %d files, want %d", len(files), len(want))
		}
	})
}

// Hidden directories are searched; Find carries no ignore rules.
func TestFindSearchesHiddenAndUnusualDirectories(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{
		".hidden/package.json":          "{}",
		".git/package.json":             "{}",
		"dir with space/ü/package.json": "{}",
		"*.json":                        "{}",
	})
	got := paths(root, find(t, root, "package.json", "*.json"))
	want := []string{".git/package.json", ".hidden/package.json", "*.json", "dir with space/ü/package.json"}
	if !slices.Equal(got, want) {
		t.Fatalf("Find = %v, want %v", got, want)
	}
}

// A symlink is not a regular file, even when it points at one.
func TestFindOmitsSymlinksToFiles(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"real/package.json": "{}", "target.txt": "x"})
	if err := os.Symlink(filepath.Join(root, "real", "package.json"), filepath.Join(root, "alias-package.json")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(root, "real", "pnpm-lock.yaml")); err != nil {
		t.Fatal(err)
	}
	got := paths(root, find(t, root, "package.json", "alias-package.json", "pnpm-lock.yaml"))
	if !slices.Equal(got, []string{"real/package.json"}) {
		t.Fatalf("Find = %v, want only the regular file", got)
	}
}

// A symlink named like a searched name that points at a directory is
// neither returned nor followed.
func TestFindOmitsSymlinkToDirectoryNamedLikeTarget(t *testing.T) {
	work := t.TempDir()
	root, outside := filepath.Join(work, "root"), filepath.Join(work, "outside")
	plant(t, root, map[string]string{"keep/package.json": "{}"})
	plant(t, outside, map[string]string{"package.json": "{}"})
	if err := os.Symlink(outside, filepath.Join(root, "package.json")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	got := paths(root, find(t, root, "package.json"))
	if !slices.Equal(got, []string{"keep/package.json"}) {
		t.Fatalf("Find = %v", got)
	}
}

// An unreadable root is a permission error, not an empty result.
func TestFindOfAnUnreadableRootIsPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lists mode-000 directories")
	}
	root := filepath.Join(t.TempDir(), "locked")
	plant(t, root, map[string]string{"package.json": "{}"})
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.Find(ctx, root, "package.json")
		return err
	})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Find under an unreadable root = %v, want fs.ErrPermission", err)
	}
}

// Find reports the filesystem as it is now: no result outlives its search.
func TestFindSeesChangesBetweenSearches(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"a/package.json": "{}"})
	if got := paths(root, find(t, root, "package.json")); !slices.Equal(got, []string{"a/package.json"}) {
		t.Fatalf("first Find = %v", got)
	}
	plant(t, root, map[string]string{"b/package.json": "{}"})
	if got := paths(root, find(t, root, "package.json")); !slices.Equal(got, []string{"a/package.json", "b/package.json"}) {
		t.Fatalf("Find after a create = %v", got)
	}
	if err := os.Remove(filepath.Join(root, "a", "package.json")); err != nil {
		t.Fatal(err)
	}
	if got := paths(root, find(t, root, "package.json")); !slices.Equal(got, []string{"b/package.json"}) {
		t.Fatalf("Find after a delete = %v", got)
	}
}

// A File that File.Write just established is discoverable.
func TestFindDiscoversFilesWrittenThroughFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "new", "deep", "package.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	var got []evo.File
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := (evo.File{Path: target, Content: evo.Bytes("{}")}).Write(ctx); err != nil {
			return err
		}
		var err error
		got, err = evo.Find(ctx, root, "package.json")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != target {
		t.Fatalf("Find = %v, want [%s]", got, target)
	}
}

// Discovery leaves no trace in the searched tree: no lock file, marker, or
// temporary, and no entry's mode or mtime changes.
func TestFindLeavesTheSearchedTreeUntouched(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	before := snapshot(t, root)
	find(t, root, "package.json", "pnpm-lock.yaml")
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Fatalf("Find changed the searched tree:\nbefore %v\nafter  %v", before, after)
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[path] = info.Mode().String() + info.ModTime().String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Coalesced callers each own their result: mutating one caller's slice
// must not change another's.
func TestFindCoalescedResultsDoNotAlias(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	const searchers = 4
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(),
	})
	group := out.Group("alias")
	results := make([][]evo.File, searchers)
	for i := range searchers {
		group.Task("find " + string(rune('a'+i))).Define(func(ctx context.Context) error {
			var err error
			results[i], err = evo.Find(ctx, root, "package.json")
			return err
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("group: %v", err)
	}
	_ = out.Finish()
	want := slices.Clone(results[0])
	for i := range results[1:] {
		for j := range results[i+1] {
			results[i+1][j] = evo.File{Path: "scribbled"}
		}
	}
	if !slices.EqualFunc(results[0], want, func(a, b evo.File) bool { return a.Path == b.Path }) {
		t.Fatalf("one caller's mutation leaked into another's result: %v", results[0])
	}
}

func TestFindRequiresAtLeastOneName(t *testing.T) {
	root := t.TempDir()
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.Find(ctx, root)
		return err
	})
	if !errors.Is(err, evo.ErrFindNamesMissing) {
		t.Fatalf("Find with no names = %v, want ErrFindNamesMissing", err)
	}
}

func TestFindOfAMissingRootIsNotExist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.Find(ctx, root, "package.json")
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Find under a missing root = %v, want fs.ErrNotExist", err)
	}
}

func TestFindDoesNotFollowSymlinkedDirectories(t *testing.T) {
	work := t.TempDir()
	root, outside := filepath.Join(work, "root"), filepath.Join(work, "outside")
	plant(t, root, map[string]string{"package.json": "{}"})
	plant(t, outside, map[string]string{"package.json": "{}"})
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	got := paths(root, find(t, root, "package.json"))
	if !slices.Equal(got, []string{"package.json"}) {
		t.Fatalf("Find = %v, want only the entry inside the root", got)
	}
}

func TestFindHonorsContextCancellation(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := evo.Find(ctx, root, "package.json")
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Find = %v, want context.Canceled", err)
	}
}

func TestFindHonorsADeadline(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()
		_, err := evo.Find(ctx, root, "package.json")
		return err
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired Find = %v, want context.DeadlineExceeded", err)
	}
}

// Concurrent Finds and a concurrent File.Write under the same root all
// complete: Find takes no tree-wide lock that a Write would have to wait
// on, and a Write takes no lock that blocks discovery.
func TestFindNeverTakesATreeWideLock(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(),
	})
	group := out.Group("concurrent")
	const searchers = 4
	results := make([][]evo.File, searchers)
	for i := range searchers {
		group.Task("find " + string(rune('a'+i))).Define(func(ctx context.Context) error {
			var err error
			results[i], err = evo.Find(ctx, root, "package.json")
			return err
		})
	}
	target := filepath.Join(root, "packages", "a", "src", "index.ts")
	group.Task("write").Define(func(ctx context.Context) error {
		return evo.File{Path: target, Content: evo.Bytes("export const x = 1\n")}.Write(ctx)
	})
	done := make(chan error, 1)
	go func() { done <- group.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("group: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Find and Write under one root did not both complete")
	}
	_ = out.Finish()
	for i, files := range results {
		if len(files) != 5 {
			t.Fatalf("searcher %d found %d files, want 5", i, len(files))
		}
	}
	if got, _ := os.ReadFile(target); string(got) != "export const x = 1\n" {
		t.Fatalf("the concurrent Write did not land: %q", got)
	}
}

// Identical concurrent searches share one traversal and agree on results.
func TestFindCoalescesIdenticalConcurrentSearches(t *testing.T) {
	root := t.TempDir()
	plant(t, root, workspace)
	const searchers = 6
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(),
	})
	group := out.Group("coalesced")
	var mu sync.Mutex
	results := make([][]string, 0, searchers)
	for i := range searchers {
		group.Task("find " + string(rune('a'+i))).Define(func(ctx context.Context) error {
			files, err := evo.Find(ctx, root, "package.json", "pnpm-lock.yaml")
			if err != nil {
				return err
			}
			mu.Lock()
			results = append(results, paths(root, files))
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("group: %v", err)
	}
	_ = out.Finish()
	if len(results) != searchers {
		t.Fatalf("%d of %d searches reported", len(results), searchers)
	}
	for _, r := range results[1:] {
		if !slices.Equal(r, results[0]) {
			t.Fatalf("coalesced searches disagree: %v vs %v", r, results[0])
		}
	}
}
