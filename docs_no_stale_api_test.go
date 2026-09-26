package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// Every rules.Symbol with a Taught pattern is checked (spec §46's public
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
	"CHANGELOG.md",
}

// staleAPIHistoricalFragments mark frozen/verbatim documents — old design
// specs, ADRs, and the input spec copy — that describe past or externally
// authored state rather than teaching the current API. version_drift_test.go
// exempts the same class of path for the same reason.
var staleAPIHistoricalFragments = []string{
	"docs/architecture/",
	"docs/adr/",
	"docs/acceptance/reference/",
	// The v0.2.8-era planning basis and its polish synthesis: dated design
	// history, not current API. Both are explicitly labeled "Historical".
	"docs/roadmap/implementation-basis.md",
	"docs/roadmap/polish-synthesis.md",
	"/COMPLETENESS_",
}

// TestRemovedInPatternAcceptsOnlyCHANGELOGBareTense pins the one asymmetry
// in the migration-note check: CHANGELOG.md's "## Removed" bullets may use
// the bare "was removed"/"renamed" past tense because the section heading
// itself already carries the release, but every other doc must still name
// the release explicitly — a stray "renamed" in, say, docs/reference.md
// must not excuse teaching a retired name there.
func TestRemovedInPatternAcceptsOnlyCHANGELOGBareTense(t *testing.T) {
	cases := []struct {
		rel     string
		text    string
		release rules.Release
		want    bool
	}{
		{"CHANGELOG.md", "TaskHandle.Record was removed", rules.Release1_1, true},
		{"CHANGELOG.md", "TaskHandle.Add renamed to Effect", rules.Release1_1, true},
		{"CHANGELOG.md", "no migration note here", rules.Release1_1, false},
		{"docs/reference.md", "TaskHandle.Record was removed", rules.Release1_1, false},
		{"docs/reference.md", "TaskHandle.Add renamed to Effect", rules.Release1_1, false},
		{"docs/reference.md", "TaskHandle.Record removed in 1.1", rules.Release1_1, true},
	}
	for _, c := range cases {
		if got := removedInPattern(c.rel, c.release).MatchString(c.text); got != c.want {
			t.Errorf("removedInPattern(%q, %s).MatchString(%q) = %v, want %v", c.rel, c.release, c.text, got, c.want)
		}
	}
}

func TestDocsCarryNoStaleAPI(t *testing.T) {
	root := moduleRoot(t)
	docs, offsets := currentDocs(t, root)
	for path, body := range docs {
		checkNoUnexplainedStaleAPI(t, path, body, offsets[path])
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
	docs, _ := currentDocs(t, root)
	for rel, body := range docs {
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
// module-relative path, plus every MCP catalog guide body (served to
// agents, never a file); frozen historical documents are skipped. The
// second return value gives each path's line-number offset: how many lines
// were cut from the start of the real file before the returned body began,
// so a caller can translate a body-relative line back to the file's own
// numbering.
func currentDocs(t *testing.T, root string) (map[string]string, map[string]int) {
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
	offsets := map[string]int{}
	for path := range files {
		rel, _ := filepath.Rel(root, path)
		if isStaleAPIHistorical(rel) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(body)
		if filepath.Base(rel) == "CHANGELOG.md" {
			// Every already-released version section is a dated record of
			// what that release changed, in the same frozen-history class
			// as docs/architecture and the ADRs: it teaches nothing about
			// today's live surface. Only "## Unreleased" — the section
			// about the release this branch is building — is scanned.
			var section string
			section, offsets[rel] = unreleasedSection(text)
			text = section
		}
		docs[rel] = text
	}
	for _, guide := range catalog.All() {
		docs["internal/agent/catalog guide "+guide.ID] = guide.Body
	}
	return docs, offsets
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

func checkNoUnexplainedStaleAPI(t *testing.T, rel, body string, lineOffset int) {
	t.Helper()
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		for _, hit := range rules.TaughtIn(line) {
			if allowedNearby(lines, i, removedInPattern(rel, hit.Symbol.RemovedIn)) {
				continue
			}
			t.Errorf("%s:%d: retired API %q taught without a nearby \"removed in %s\" migration note (use %s):\n%s",
				rel, i+1+lineOffset, hit.Match, hit.Symbol.RemovedIn, hit.Symbol.Replacement, line)
		}
	}
}

// removedInPattern is the migration-note marker for release: a retired
// symbol is only legitimate within staleAPIWindow lines of it. Every doc
// accepts the release-qualified "removed in X" phrasing (docs/migration/*.md
// section headers). CHANGELOG.md alone also accepts the bare past-tense
// "was/were removed"/"renamed" phrasing its dated "### Removed" bullets
// already use, because that section's own "## <version>" heading (or, for
// the in-progress release, the fact that only "## Unreleased" is scanned at
// all) already carries the release — every other doc still needs the
// symbol's own release named, so a stray "renamed" elsewhere can't excuse
// teaching a retired name as current.
func removedInPattern(rel string, release rules.Release) *regexp.Regexp {
	pattern := `(?i)removed in ` + regexp.QuoteMeta(string(release))
	if filepath.Base(rel) == "CHANGELOG.md" {
		pattern += `|\b(?:was|were) removed\b|\brenamed\b`
	}
	return regexp.MustCompile(pattern)
}

// allowedNearby reports whether allow matches line i or any of the
// staleAPIWindow lines before or after it.
func allowedNearby(lines []string, i int, allow *regexp.Regexp) bool {
	start := max(i-staleAPIWindow, 0)
	end := min(i+staleAPIWindow+1, len(lines))
	return slices.ContainsFunc(lines[start:end], allow.MatchString)
}
