//go:build unix

package tree_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

var _ func(evo.Tree, context.Context, string, ...evo.ReplaceOption) (evo.ReplaceResult, error) = evo.Tree.ReplaceTree

// pruneSwapFixture is a project copy of a canonical store tree: dest holds
// the same bytes as canonical in its own private files, and expected is
// dest's digest (equal to canonical's).
type pruneSwapFixture struct {
	canonical, parent, dest, expected string
}

func newPruneSwapFixture(t *testing.T) pruneSwapFixture {
	t.Helper()
	canonical := cloneSource(t)
	parent := t.TempDir()
	dest := filepath.Join(parent, "pkg")
	pruneByteCopy(t, canonical, dest)
	expected := pruneChecksum(t, dest)
	if want := pruneChecksum(t, canonical); expected != want {
		t.Fatalf("fixture: private copy digests to %s, canonical to %s", expected, want)
	}
	return pruneSwapFixture{canonical: canonical, parent: parent, dest: dest, expected: expected}
}

// swap replaces dest with a clone of canonical under opts.
func (f pruneSwapFixture) swap(t *testing.T, opts ...evo.ReplaceOption) (evo.ReplaceResult, error) {
	t.Helper()
	var got evo.ReplaceResult
	tree := evo.Tree{Path: f.dest, Content: evo.Clone{From: evo.Tree{Path: f.canonical}}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		got, err = tree.ReplaceTree(ctx, f.expected, opts...)
		return err
	})
	return got, err
}

// pruneByteCopy copies the tree at from to to byte by byte (no clone), as
// npm installing a private copy would.
func pruneByteCopy(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		target := filepath.Join(to, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0o755)
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pruneInode(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return uint64(info.Sys().(*syscall.Stat_t).Ino)
}

func TestPrune_ReplaceTreeOfAnIdenticalDestinationIsSatisfiedByDefault(t *testing.T) {
	f := newPruneSwapFixture(t)
	before := pruneInode(t, f.dest)
	got, err := f.swap(t)
	if err != nil {
		t.Fatalf("ReplaceTree = %v", err)
	}
	if got.Published {
		t.Fatalf("ReplaceTree of an identical destination = Published, want satisfied (Published=false)")
	}
	if after := pruneInode(t, f.dest); after != before {
		t.Fatalf("satisfied ReplaceTree changed the destination inode %d -> %d", before, after)
	}
}

func TestPrune_ReplaceTreeRepublishSwapsInTheCloneOfAnIdenticalDestination(t *testing.T) {
	f := newPruneSwapFixture(t)
	before := pruneInode(t, f.dest)
	got, err := f.swap(t, evo.Republish())
	if err != nil {
		t.Fatalf("ReplaceTree(Republish) = %v", err)
	}
	if !got.Published {
		t.Fatalf("ReplaceTree(Republish) = not Published, want the clone swapped in")
	}
	if after := pruneInode(t, f.dest); after == before {
		t.Fatalf("Republish kept the destination inode %d; the clone was never swapped in", before)
	}
	if sum := pruneChecksum(t, f.dest); sum != f.expected {
		t.Fatalf("republished destination digests to %s, want %s", sum, f.expected)
	}
	if names := advNames(t, f.parent); !slices.Equal(names, []string{"pkg"}) {
		t.Fatalf("Republish left %v in the parent; the private copy must be gone", names)
	}
}

func TestPrune_ReplaceTreeRepublishOfAnEditedDestinationIsTreeChanged(t *testing.T) {
	f := newPruneSwapFixture(t)
	plant(t, f.dest, map[string]string{"lib/a.js": "local edit"})
	got, err := f.swap(t, evo.Republish())
	if !errors.Is(err, evo.ErrTreeChanged) {
		t.Fatalf("ReplaceTree(Republish) after a local edit = %v, want ErrTreeChanged", err)
	}
	if got.Published {
		t.Fatalf("a refused ReplaceTree reported Published")
	}
	if body, _ := os.ReadFile(filepath.Join(f.dest, "lib/a.js")); string(body) != "local edit" {
		t.Fatalf("destination lib/a.js = %q, want the local edit untouched", body)
	}
}

func TestPrune_ReplaceTreeRepublishesUnrelatedTreesConcurrently(t *testing.T) {
	fixtures := []pruneSwapFixture{newPruneSwapFixture(t), newPruneSwapFixture(t)}
	results := make([]evo.ReplaceResult, len(fixtures))
	errs := make([]error, len(fixtures))
	var wg sync.WaitGroup
	for i, f := range fixtures {
		wg.Go(func() { results[i], errs[i] = f.swap(t, evo.Republish()) })
	}
	wg.Wait()
	for i, f := range fixtures {
		if errs[i] != nil || !results[i].Published {
			t.Fatalf("concurrent Republish %d = %+v, %v; want Published", i, results[i], errs[i])
		}
		if sum := pruneChecksum(t, f.dest); sum != f.expected {
			t.Fatalf("concurrent Republish %d digests to %s, want %s", i, sum, f.expected)
		}
	}
}
