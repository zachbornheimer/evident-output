package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

// evoIdentifier matches an evo.<Exported> reference in prose or code.
var evoIdentifier = regexp.MustCompile(`\bevo\.([A-Z]\w*)`)

// goldenDeclName matches the declared name on one api_golden.txt line.
var goldenDeclName = regexp.MustCompile(`^(?:func|type|value) (?:\(\w+\) )?(\w+)`)

// TestMigrationGuidesNameOnlyRealAPI fails when a migration guide names an
// evo identifier that is neither in the live API contract nor a retired
// name — the evo.PatchSpec class of defect, where a guide tells the reader
// to call something that never existed.
func TestMigrationGuidesNameOnlyRealAPI(t *testing.T) {
	root := moduleRoot(t)
	known := liveAPINames(t, root)
	for _, name := range rules.ContractNames() {
		base := strings.TrimSuffix(name[strings.LastIndex(name, ".")+1:], "(")
		known[base] = true
	}
	guides, err := filepath.Glob(filepath.Join(root, "docs", "migration", "*.md"))
	if err != nil || len(guides) == 0 {
		t.Fatalf("no migration guides found: %v", err)
	}
	for _, guide := range guides {
		body, err := os.ReadFile(guide)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, guide)
		for i, line := range strings.Split(string(body), "\n") {
			for _, m := range evoIdentifier.FindAllStringSubmatch(line, -1) {
				if !known[m[1]] {
					t.Errorf("%s:%d: evo.%s is not in the API contract or the retired table", rel, i+1, m[1])
				}
			}
		}
	}
}

// liveAPINames is every name the live API contract declares.
func liveAPINames(t *testing.T, root string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	golden, err := os.ReadFile(filepath.Join(root, apisurface.GoldenRelPath))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(golden), "\n") {
		if m := goldenDeclName.FindStringSubmatch(line); m != nil {
			names[m[1]] = true
		}
	}
	return names
}

// releasedGoldenRelPath is the API contract as of PublishedRelease;
// cut-release refreshes it from api_golden.txt at every tag.
const releasedGoldenRelPath = "testdata/api_golden_released.txt"

// TestChangelogCoversEveryAPIChange fails when an exported declaration was
// added, removed, or changed since the last release but CHANGELOG.md's
// Unreleased section never names it, so the changelog stays mechanically
// complete rather than hand-remembered.
func TestChangelogCoversEveryAPIChange(t *testing.T) {
	root := moduleRoot(t)
	current := readLineSet(t, filepath.Join(root, apisurface.GoldenRelPath))
	released := readLineSet(t, filepath.Join(root, releasedGoldenRelPath))
	changelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	unreleased, _ := unreleasedSection(string(changelog))
	for line := range symmetricDifference(current, released) {
		m := goldenDeclName.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(m[1]) + `\b`).MatchString(unreleased) {
			t.Errorf("CHANGELOG.md Unreleased never names %s, changed since the last release: %s", m[1], line)
		}
	}
}

func readLineSet(t *testing.T, path string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set
}

func symmetricDifference(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for line := range a {
		if !b[line] {
			out[line] = true
		}
	}
	for line := range b {
		if !a[line] {
			out[line] = true
		}
	}
	return out
}

// unreleasedSection is the text between "## Unreleased" and the next
// "## " heading, plus how many lines of changelog precede that text — so a
// caller reporting a line number found inside the returned section can add
// this back to get the real CHANGELOG.md line number.
func unreleasedSection(changelog string) (string, int) {
	before, rest, ok := strings.Cut(changelog, "\n## Unreleased\n")
	if !ok {
		return "", 0
	}
	offset := strings.Count(before, "\n") + 2 // the cut "\n## Unreleased\n" line itself, plus 1 to land on rest's first line
	if section, _, ok0 := strings.Cut(rest, "\n## "); ok0 {
		return section, offset
	}
	return rest, offset
}
