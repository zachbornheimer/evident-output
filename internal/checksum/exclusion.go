package checksum

import (
	"fmt"
	"regexp"
)

// Exclusion is a set of path patterns a tree digest leaves out. Each
// pattern is a Go regexp matched, unanchored, against "/" plus the
// slash-separated path inside the tree; a directory's path ends in "/".
// The tree's own root is never matched. The zero Exclusion excludes
// nothing.
type Exclusion struct{ patterns []*regexp.Regexp }

// CompileExclusion compiles patterns into one Exclusion (their union). An
// invalid pattern is ErrInvalidExclude: silently excluding nothing would
// report a digest the caller did not ask for.
func CompileExclusion(patterns ...string) (Exclusion, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return Exclusion{}, fmt.Errorf("%w %q: %w", ErrInvalidExclude, pattern, err)
		}
		compiled = append(compiled, re)
	}
	return Exclusion{patterns: compiled}, nil
}

// excludes reports whether the in-tree path rel ("/a/b", or "/a/" for a
// directory) is left out.
func (e Exclusion) excludes(rel string) bool {
	for _, re := range e.patterns {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}
