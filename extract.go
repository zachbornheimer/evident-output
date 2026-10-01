package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

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
