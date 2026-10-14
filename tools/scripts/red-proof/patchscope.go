package main

import (
	"slices"
	"strings"
)

const (
	patchTargetPrefix = "+++ b/"
	patchSourcePrefix = "--- a/"
	patchNullTarget   = "+++ /dev/null"
	testFileSuffix    = "_test.go"
)

// patchPaths lists the files a unified diff writes, sorted: each `+++ b/`
// target, and the `--- a/` source of a file the diff deletes.
func patchPaths(patch []byte) []string {
	var paths []string
	var source string
	for line := range strings.SplitSeq(string(patch), "\n") {
		switch {
		case strings.HasPrefix(line, patchSourcePrefix):
			source = diffPath(strings.TrimPrefix(line, patchSourcePrefix))
		case strings.HasPrefix(line, patchTargetPrefix):
			paths = append(paths, diffPath(strings.TrimPrefix(line, patchTargetPrefix)))
		case line == patchNullTarget && source != "":
			paths = append(paths, source)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// diffPath drops the tab-separated timestamp some diff tools append to a path.
func diffPath(field string) string {
	path, _, _ := strings.Cut(field, "\t")
	return path
}

// outsideFix returns the patched files that the fix commit did not change.
// Test files are not part of a fix's source, so a patch may not target one
// either: it re-introduces the bug, and the guarding tests must stay intact.
func outsideFix(patched, fixChanged []string) []string {
	var outside []string
	for _, path := range patched {
		if !slices.Contains(fixChanged, path) || strings.HasSuffix(path, testFileSuffix) {
			outside = append(outside, path)
		}
	}
	return outside
}
