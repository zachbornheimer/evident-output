package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// ChecksumOption adjusts a Tree checksum or comparison. Obtain one from
// Exclude.
type ChecksumOption struct{ exclude string }

// Exclude drops every tree entry whose path matches pattern, a Go regexp
// matched (unanchored) against "/" plus the slash-separated path inside the
// tree; directories carry a trailing "/". `.*\/\.git\/.*` drops a .git
// subtree at any depth. Several Exclude options combine.
func Exclude(pattern string) ChecksumOption { return ChecksumOption{exclude: pattern} }

// Checksum returns the lowercase hex SHA-256 of the file's bytes. It is the
// same digest Tree.Checksum uses for this file as a leaf.
func (f File) Checksum(ctx context.Context) (string, error) {
	return engine.FileChecksum(ctx, f.Path)
}

// Checksum returns the tree's Merkle digest in lowercase hex, built from
// each file's File.Checksum, its path, and the tree structure.
func (t Tree) Checksum(ctx context.Context, opts ...ChecksumOption) (string, error) {
	return engine.TreeChecksum(ctx, t.Path, excludePatterns(opts))
}

// excludePatterns is the Exclude patterns opts carry, in order.
func excludePatterns(opts []ChecksumOption) []string {
	patterns := make([]string, 0, len(opts))
	for _, opt := range opts {
		patterns = append(patterns, opt.exclude)
	}
	return patterns
}
