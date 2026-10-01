package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
	"github.com/zachbornheimer/evident-output/internal/publish"
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

// Recover settles Path after a Replace with the same Content and expected
// was interrupted, deciding by digest alone, never by a leftover's name,
// age, or presence. Under Path's coordination it digests Path and the
// hidden trees the interrupted Replace left beside it:
//
//   - Path holds expected (RecoverIntact) or Content (RecoverCompletedReplacement):
//     leftovers holding either are deleted; any other leftover is kept and
//     listed.
//   - Path is missing and a leftover holds expected: it is put back at
//     Path (RecoverRestoredOriginal); leftovers holding either are deleted.
//   - Otherwise (Path holds something else, or is missing with no original
//     to restore): RecoverUnrecoverable, an error wrapping ErrTreeChanged,
//     every leftover listed, and nothing changed.
//
// Recover deletes only trees it proved redundant, and never a stage a live
// writer still owns (those are listed). A nil Content means the
// replacement is unknown: only Path holding expected settles it, and
// leftovers that are not expected are kept. Like Write, it runs only
// inside a Task (else ErrNoTaskContext); under DryRun it reports the
// decision and changes nothing.
func (t Tree) Recover(ctx context.Context, expected string) (RecoverResult, error) {
	src, err := t.source()
	if err != nil {
		return RecoverResult{State: RecoverUnrecoverable}, err
	}
	got, err := engine.TreeRecover(ctx, t.Path, src, expected)
	return RecoverResult{State: recoverStates[got.Outcome], Leftovers: got.Kept}, err
}

// RecoverResult is how Recover settled a Tree.
type RecoverResult struct {
	State RecoverState
	// Leftovers are the hidden trees beside Path that Recover kept: their
	// digest is neither expected nor Content's, or (RecoverUnrecoverable)
	// Path was not proven whole.
	Leftovers []string
}

// RecoverState is the state Recover left a Tree's Path in.
type RecoverState int

// Recover states.
const (
	// RecoverIntact: Path holds the expected (original) tree.
	RecoverIntact RecoverState = iota + 1
	// RecoverRestoredOriginal: Path was missing and the original was put
	// back.
	RecoverRestoredOriginal
	// RecoverCompletedReplacement: Path holds Content.
	RecoverCompletedReplacement
	// RecoverUnrecoverable: neither tree could be established at Path;
	// nothing was changed.
	RecoverUnrecoverable
)

var recoverStates = map[publish.Outcome]RecoverState{
	publish.OutcomeIntact:               RecoverIntact,
	publish.OutcomeRestoredOriginal:     RecoverRestoredOriginal,
	publish.OutcomeCompletedReplacement: RecoverCompletedReplacement,
	publish.OutcomeUnrecoverable:        RecoverUnrecoverable,
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
