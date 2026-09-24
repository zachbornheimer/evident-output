package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

// Every retired.Symbol with a Taught pattern is checked (spec §46's public
// API drift test, mirrored here for prose: docs, README, doc.go, and the
// agent sections corpus). A hit is only legitimate inside a note that says
// "removed in <release>" for that symbol's release — teaching it as
// current, live surface is the defect this test exists to catch.

// staleAPIWindow is how many lines before AND after the hit line (inclusive
// of the hit line itself) are searched for the allow phrase — enough to
// cover a heading or lead sentence that precedes, or a trailing clause that
// follows, a fenced before/after code example.
const staleAPIWindow = 8

// staleAPIScanRoots are scanned recursively; staleAPIScanFiles are scanned
// directly regardless of extension.
var staleAPIScanRoots = []string{
	"docs",
	"internal/agent/sections",
	"skills",
}

var staleAPIScanFiles = []string{
	"README.md",
	"AGENTS.md",
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
	// The v0.2.8-era planning basis: dated design history, not current API.
	"docs/roadmap/implementation-basis.md",
	"/COMPLETENESS_",
}

func TestDocsCarryNoStaleAPI(t *testing.T) {
	root := moduleRoot(t)
	for path, body := range currentDocs(t, root) {
		checkNoUnexplainedStaleAPI(t, path, body)
	}
}

// unimplementedClaim marks prose that calls something not built yet.
var unimplementedClaim = regexp.MustCompile(`(?i)not yet implemented|\(planned[;)]`)

// phantomAPI is prose describing API that never existed: freshness inputs
// are FileSpec.Basis / ExecSpec.Basis, never a Task-level Basis.
var phantomAPI = regexp.MustCompile(`(?i)\bTask-level Basis\b|\bTask's Basis\b`)

// TestDocsNeverCallLiveAPIUnimplemented fails when current docs call an
// exported identifier "planned" or "not yet implemented" (doc.go once said
// that of evo.Exec, which exists) or describe API that never existed.
func TestDocsNeverCallLiveAPIUnimplemented(t *testing.T) {
	root := moduleRoot(t)
	live := liveAPINames(t, root)
	for rel, body := range currentDocs(t, root) {
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			if m := phantomAPI.FindString(line); m != "" {
				t.Errorf("%s:%d: %q describes API that does not exist (FileSpec.Basis / ExecSpec.Basis):\n%s", rel, i+1, m, line)
			}
			if !unimplementedClaim.MatchString(line) {
				continue
			}
			// A wrapped sentence names its subject a line earlier.
			subject := strings.Join(lines[max(i-1, 0):i+1], " ")
			for _, m := range evoIdentifier.FindAllStringSubmatch(subject, -1) {
				if live[m[1]] {
					t.Errorf("%s:%d: calls live API evo.%s unimplemented:\n%s", rel, i+1, m[1], line)
				}
			}
		}
	}
}

// currentDocs reads every doc that teaches the current API, keyed by its
// module-relative path; frozen historical documents are skipped.
func currentDocs(t *testing.T, root string) map[string]string {
	t.Helper()
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

	docs := map[string]string{}
	for path := range files {
		rel, _ := filepath.Rel(root, path)
		if isStaleAPIHistorical(rel) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		docs[rel] = string(body)
	}
	return docs
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
		for _, hit := range retired.TaughtIn(line) {
			if allowedNearby(lines, i, removedInPattern(hit.Symbol.RemovedIn)) {
				continue
			}
			t.Errorf("%s:%d: retired API %q taught without a nearby \"removed in %s\" migration note (use %s):\n%s",
				rel, i+1, hit.Match, hit.Symbol.RemovedIn, hit.Symbol.Replacement, line)
		}
	}
}

// removedInPattern is the migration-note marker for release: a retired
// symbol is only legitimate within staleAPIWindow lines of it.
func removedInPattern(release retired.Release) *regexp.Regexp {
	return regexp.MustCompile(`(?i)removed in ` + regexp.QuoteMeta(string(release)))
}

// allowedNearby reports whether allow matches line i or any of the
// staleAPIWindow lines before or after it.
func allowedNearby(lines []string, i int, allow *regexp.Regexp) bool {
	start := max(i-staleAPIWindow, 0)
	end := min(i+staleAPIWindow+1, len(lines))
	return slices.ContainsFunc(lines[start:end], allow.MatchString)
}
