package hillclimb

import (
	"path"
	"strings"
)

const testFileSuffix = "_test.go"

// FrozenGlobs are never edited by a step: the eval harness, the grader and
// every task prompt, expectation, fixture and trap (all under evaltask), the
// conformance suite, and the public API goldens.
var FrozenGlobs = []string{
	"eval/**",
	"internal/agent/evaltask/**",
	"conformance/**",
	"testdata/api_golden*.txt",
	".evor/**",
}

// TunedGlobs are the only paths a step may edit: guides, the reference
// examples, rule and review message text, and the runtime misuse error
// strings. Go files among them may change string literals only (see
// StringLiteralsOnly), which is what keeps scheduler semantics frozen.
var TunedGlobs = []string{
	"docs/guides/**",
	"docs/reference.md",
	"internal/agent/rules/*.go",
	"internal/agent/review/*.go",
	"internal/engine/errors.go",
	"problem.go",
}

// MatchGlob reports whether p matches pattern, where "**" spans any number of
// path segments and every other segment follows path.Match.
func MatchGlob(pattern, p string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

func matchSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(segments); skip++ {
			if matchSegments(pattern[1:], segments[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segments) == 0 {
		return false
	}
	if ok, _ := path.Match(pattern[0], segments[0]); !ok {
		return false
	}
	return matchSegments(pattern[1:], segments[1:])
}

func matchesAny(globs []string, p string) bool {
	for _, glob := range globs {
		if MatchGlob(glob, p) {
			return true
		}
	}
	return false
}

// CheckPaths returns one violation per changed path that is frozen, a test
// file, or outside the tuned surface. Frozen wins over tuned.
func CheckPaths(changed []string) []string {
	var violations []string
	for _, p := range changed {
		switch {
		case matchesAny(FrozenGlobs, p):
			violations = append(violations, p+": frozen file changed")
		case strings.HasSuffix(p, testFileSuffix):
			violations = append(violations, p+": test files are frozen")
		case !matchesAny(TunedGlobs, p):
			violations = append(violations, p+": outside the tuned surface")
		}
	}
	return violations
}
