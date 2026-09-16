package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// staleAPISymbols are spellings retired in 1.0 (spec §46's public API drift
// test, mirrored here for prose: docs, README, doc.go, and the agent
// sections corpus). A hit is only legitimate inside a note that says the
// symbol was removed — teaching it as current, live surface is the defect
// this test exists to catch.
var staleAPISymbols = []string{
	"MainWith",
	".Each(",
	"Task.Run",
	"Task.Go",
	"DisplayGroup",
	"Group.Done",
	"Sequence.Fail",
	"TaskConfig",
}

// staleAPIAllowPattern is the migration-note marker: a stale symbol is only
// legitimate within staleAPIWindow lines of this phrase.
var staleAPIAllowPattern = regexp.MustCompile(`(?i)removed in 1\.0`)

// staleAPIWindow is how many preceding lines (inclusive of the hit line
// itself) are searched for the allow phrase — enough to cover a heading or
// lead sentence followed by a fenced before/after code example.
const staleAPIWindow = 8

// staleAPIScanRoots are scanned recursively; staleAPIScanFiles are scanned
// directly regardless of extension.
var staleAPIScanRoots = []string{
	"docs",
	"internal/agent/sections",
}

var staleAPIScanFiles = []string{
	"README.md",
	"doc.go",
}

// staleAPIHistoricalFragments mark frozen/verbatim documents — old design
// specs, ADRs, and the input spec copy — that describe past or externally
// authored state rather than teaching the current API. version_drift_test.go
// exempts the same class of path for the same reason.
var staleAPIHistoricalFragments = []string{
	"docs/architecture/",
	"docs/adr/",
	"docs/acceptance/reference/",
	"/COMPLETENESS_",
}

func TestDocsCarryNoStaleAPI(t *testing.T) {
	root := moduleRoot(t)

	files := map[string]struct{}{}
	for _, f := range staleAPIScanFiles {
		files[filepath.Join(root, f)] = struct{}{}
	}
	for _, dir := range staleAPIScanRoots {
		base := filepath.Join(root, dir)
		if err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			files[path] = struct{}{}
			return nil
		}); err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}

	for path := range files {
		rel, _ := filepath.Rel(root, path)
		if isStaleAPIHistorical(rel) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		checkNoUnexplainedStaleAPI(t, rel, string(body))
	}
}

func isStaleAPIHistorical(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, frag := range staleAPIHistoricalFragments {
		if strings.Contains(rel, frag) {
			return true
		}
	}
	return false
}

func checkNoUnexplainedStaleAPI(t *testing.T, rel, body string) {
	t.Helper()
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		for _, symbol := range staleAPISymbols {
			if !strings.Contains(line, symbol) {
				continue
			}
			if !staleAPIAllowedNearby(lines, i) {
				t.Errorf("%s:%d: stale API %q taught without a nearby \"removed in 1.0\" migration note:\n%s",
					rel, i+1, symbol, line)
			}
		}
	}
}

// staleAPIAllowedNearby reports whether the allow phrase appears on line i
// or any of the staleAPIWindow lines before it.
func staleAPIAllowedNearby(lines []string, i int) bool {
	start := i - staleAPIWindow
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start : i+1] {
		if staleAPIAllowPattern.MatchString(line) {
			return true
		}
	}
	return false
}
