package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Tree is one directory and everything beneath it: where it lives and,
// optionally, what it should hold. A Tree literal performs no I/O; its
// methods do. Tree shares File's vocabulary at directory cardinality.
type Tree struct {
	// Path is required. A relative Path resolves against the Run's
	// workspace directory captured once at Run start.
	Path string
	// Content is the desired content (Extract). nil declares no desired
	// content; it never means "absent" (use Remove for that).
	Content TreeContent
}

// Read returns the regular files under the tree, sorted by path, each with
// an absolute Path and nil Content (content is read lazily).
func (t Tree) Read(ctx context.Context) ([]File, error) {
	paths, err := engine.TreeRead(ctx, t.Path)
	if err != nil {
		return nil, err
	}
	files := make([]File, len(paths))
	for i, path := range paths {
		files[i] = File{Path: path}
	}
	return files, nil
}

// Write establishes Content at Path, replacing the whole tree atomically:
// entries absent from Content are gone afterwards.
func (t Tree) Write(ctx context.Context) error {
	src, err := t.source()
	if err != nil {
		return err
	}
	return engine.TreeWrite(ctx, t.Path, src)
}

// Replace is Write that commits only if Path still holds the tree whose
// Tree.Checksum was expected. The check runs inside the destination's
// critical section, after Content is prepared, so an edit that lands
// between the caller's observation and the commit is never overwritten:
// Replace returns ErrTreeChanged and leaves Path exactly as found. A
// missing Path has changed too. The original tree is kept beside Path
// until the new one verifies, then deleted.
func (t Tree) Replace(ctx context.Context, expected string) error {
	src, err := t.source()
	if err != nil {
		return err
	}
	return engine.TreeReplace(ctx, t.Path, src, expected)
}

// Verify reports whether Path holds Content: nil, or ErrVerifyMismatch.
func (t Tree) Verify(ctx context.Context) error {
	src, err := t.source()
	if err != nil {
		return err
	}
	return engine.TreeVerify(ctx, t.Path, src)
}

// Equal reports whether t and other have the same structure and contents,
// honouring Exclude options.
func (t Tree) Equal(ctx context.Context, other Tree, opts ...ChecksumOption) (bool, error) {
	return engine.TreeEqual(ctx, t.Path, other.Path, excludePatterns(opts))
}

// source is the engine's view of t.Content: nil for no declared Content.
// Content producers supply theirs through treeSourcer.
func (t Tree) source() (engine.TreeSource, error) {
	if t.Content == nil {
		return nil, nil
	}
	sourcer, ok := t.Content.(treeSourcer)
	if !ok {
		return nil, engine.ErrContentMissing
	}
	return sourcer.treeSource(), nil
}

// treeSourcer is implemented by every TreeContent: Extract (extract.go)
// and Clone.
type treeSourcer interface{ treeSource() engine.TreeSource }

// TreeContent is a Tree's desired content: Extract or Clone. Callers can
// hold one but not implement one.
type TreeContent interface{ treeContent() }

// Clone is tree content copied from another Tree: the same structure,
// bytes, executable bits, and symlink text, so the written tree's Checksum
// equals From's. Files are cloned copy-on-write where the filesystem
// supports it and copied otherwise, never hard-linked: editing either tree
// afterwards leaves the other untouched. Only From.Path is read. A copy
// that does not digest to From's digest fails the Write and publishes
// nothing.
type Clone struct {
	From Tree
}

func (Clone) treeContent() {}

func (c Clone) treeSource() engine.TreeSource { return engine.TreeClone(c.From.Path) }

// Tree errors.
var (
	// ErrTreePathTypeMismatch is a Tree whose Path holds a non-directory.
	ErrTreePathTypeMismatch = engine.ErrTreePathTypeMismatch
	// ErrTreeChanged is a Replace whose Path no longer holds the expected
	// tree; Path is left exactly as found.
	ErrTreeChanged = engine.ErrTreeChanged
)
