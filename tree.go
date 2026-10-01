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
func (t Tree) Read(ctx context.Context) ([]File, error) { return nil, errNotImplemented }

// Write establishes Content at Path, replacing the whole tree atomically:
// entries absent from Content are gone afterwards.
func (t Tree) Write(ctx context.Context) error { return errNotImplemented }

// Verify reports whether Path holds Content: nil, or ErrVerifyMismatch.
func (t Tree) Verify(ctx context.Context) error { return errNotImplemented }

// Equal reports whether t and other have the same structure and contents,
// honouring Exclude options.
func (t Tree) Equal(ctx context.Context, other Tree, opts ...ChecksumOption) (bool, error) {
	return false, errNotImplemented
}

// TreeContent is a Tree's desired content: Extract. Callers can hold one
// but not implement one.
type TreeContent interface{ treeContent() }

// ErrTreePathTypeMismatch is a Tree whose Path holds a non-directory.
var ErrTreePathTypeMismatch = engine.ErrTreePathTypeMismatch
