package tree_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Prune replaces its own tree swap with evo.Tree, so a tree's identity must
// cover everything a package's correctness depends on, and Evo must be able
// to publish a clone of a canonical tree.

func pruneChecksum(t *testing.T, root string, opts ...evo.ChecksumOption) string {
	t.Helper()
	sum, err := evo.Tree{Path: root}.Checksum(context.Background(), opts...)
	if err != nil {
		t.Fatalf("Checksum(%s) = %v", root, err)
	}
	return sum
}

func TestPrune_ExecBitChangesTheTreeDigest(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"bin/tool": "#!/bin/sh\n", "README": "x"})
	before := pruneChecksum(t, root)
	if err := os.Chmod(filepath.Join(root, "bin/tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	if after := pruneChecksum(t, root); after == before {
		t.Fatalf("chmod +x left the tree digest unchanged (%s); a chmod'd copy must not compare equal", after)
	}
}

func TestPrune_ExcludedEntryExecBitDoesNotCount(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"bin/tool": "#!/bin/sh\n", "keep": "x"})
	exclude := evo.Exclude(`^/bin/`)
	before := pruneChecksum(t, root, exclude)
	if err := os.Chmod(filepath.Join(root, "bin/tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	if after := pruneChecksum(t, root, exclude); after != before {
		t.Fatalf("an excluded file's exec bit changed the digest: %s -> %s", before, after)
	}
}

// cloneSource is a canonical tree with nesting, an executable, and a symlink.
func cloneSource(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "canonical")
	plant(t, src, map[string]string{"package.json": "{}", "lib/a.js": "a", "lib/deep/b.js": "b", "bin/run": "#!/bin/sh\n"})
	if err := os.Chmod(filepath.Join(src, "bin/run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../lib/a.js", filepath.Join(src, "bin/a")); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestPrune_CloneYieldsAnEqualDigestAndAnIndependentTree(t *testing.T) {
	src := cloneSource(t)
	dst := filepath.Join(t.TempDir(), "project", "node_modules", "pkg")
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: dst, Content: evo.Clone{From: evo.Tree{Path: src}}}.Write); err != nil {
		t.Fatalf("Write(Clone) = %v", err)
	}
	srcSum := pruneChecksum(t, src)
	if got := pruneChecksum(t, dst); got != srcSum {
		t.Fatalf("clone digest %s != source digest %s", got, srcSum)
	}
	if target, err := os.Readlink(filepath.Join(dst, "bin/a")); err != nil || target != "../lib/a.js" {
		t.Fatalf("clone symlink = %q, %v; want the link text, never followed", target, err)
	}
	if err := os.WriteFile(filepath.Join(dst, "lib/a.js"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pruneChecksum(t, src); got != srcSum {
		t.Fatalf("editing the clone changed the source: %s -> %s", srcSum, got)
	}
	if err := contractRun(t, evo.Config{}, evo.Tree{Path: dst, Content: evo.Clone{From: evo.Tree{Path: src}}}.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify(Clone) after editing the clone = %v, want ErrVerifyMismatch", err)
	}
}

func TestPrune_CloneOfAnUnreadableSourceFailsAndLeavesNothing(t *testing.T) {
	src := cloneSource(t)
	unreadable := filepath.Join(src, "lib/deep/b.js")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })
	parent := t.TempDir()
	dst := filepath.Join(parent, "pkg")
	err := contractRun(t, evo.Config{}, evo.Tree{Path: dst, Content: evo.Clone{From: evo.Tree{Path: src}}}.Write)
	if err == nil {
		t.Fatalf("Write(Clone) of a source with an unreadable file succeeded")
	}
	if names := advNames(t, parent); len(names) != 0 {
		t.Fatalf("failed clone left %v in the destination's parent", names)
	}
}
