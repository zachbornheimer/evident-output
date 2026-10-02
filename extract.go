package evo

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/engine"
	"github.com/zachbornheimer/evident-output/internal/extract"
)

// Extract is tree content unpacked from an archive (tar.gz, tar, or zip,
// detected from content). Entries that would escape the destination,
// absolute paths, link escapes, and special files are rejected.
type Extract struct {
	// File is the archive; only its Path is read.
	File File
	// Root is an optional leading component to strip ("package").
	Root string
}

func (Extract) treeContent() {}

// Extract errors.
var (
	// ErrExtractMalformed is an empty or unreadable archive.
	ErrExtractMalformed = engine.ErrExtractMalformed
	// ErrExtractUnsafeEntry is an archive entry that is unsafe to publish.
	ErrExtractUnsafeEntry = engine.ErrExtractUnsafeEntry
)

// treeSource is the engine's view of x: Tree.Write stages the tree
// through Fill, so Tree knows no archive format.
func (x Extract) treeSource() engine.TreeSource { return extractSource{x} }

type extractSource struct{ Extract }

// Fill unpacks the archive into root and reports failures with the public
// Extract errors. The archive is read from File.Path (resolved against
// the Run's workspace); File.Content and File.Mode are ignored.
func (s extractSource) Fill(ctx context.Context, root string) error {
	if s.File.Path == "" {
		return fmt.Errorf("evo: Extract archive: %w", engine.ErrPathMissing)
	}
	err := extract.Unpack(ctx, engine.ResolvePath(ctx, s.File.Path), s.Root, root)
	switch {
	case errors.Is(err, extract.ErrMalformed):
		return fmt.Errorf("%w: %w", engine.ErrExtractMalformed, err)
	case errors.Is(err, extract.ErrUnsafeEntry):
		return fmt.Errorf("%w: %w", engine.ErrExtractUnsafeEntry, err)
	}
	return err
}
