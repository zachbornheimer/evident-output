package evo_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	changelogPath      = "CHANGELOG.md"
	migrationGuidePath = "docs/migration/2.0.md"
	removedBulletStart = "- **Removed"
	// alphaExportsPath lists what v1.2.0-alpha.1 exported: its consumers
	// upgrade to 2.0 too, and CHANGELOG.md's removed bullets stop at 1.1.
	alphaExportsPath = "testdata/v1.2.0-alpha.1_exports.txt"
)

// backtickedName matches a bare exported name in code ticks: `Name`.
var backtickedName = regexp.MustCompile("`([A-Z][A-Za-z0-9]*)`")

// identifier matches one Go identifier inside a table cell.
var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// TestMigrationGuideMapsEveryRemovedName fails when a name that 2.0 removed
// -- listed as removed by CHANGELOG.md, or exported by v1.2.0-alpha.1 and
// absent from CONTRACT.md -- never appears in the first (1.x) cell of a row of
// docs/migration/2.0.md, so a consumer who searches the guide for a 1.x name
// always finds its replacement.
func TestMigrationGuideMapsEveryRemovedName(t *testing.T) {
	removed := changelogRemovedNames(t)
	for name := range alphaExportsAbsentFromContract(t) {
		removed[name] = true
	}
	mapped := migrationGuideNames(t)

	var missing []string
	for name := range removed {
		if !mapped[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%s has no row for %d removed names (%s's removed bullets, or %s minus CONTRACT.md):\n  %s",
			migrationGuidePath, len(missing), changelogPath, alphaExportsPath, strings.Join(missing, "\n  "))
	}
}

// TestChangelogRemovedNamesAreNotContractNames fails when CHANGELOG.md lists
// as removed a name CONTRACT.md declares, such as Plain, Create, Update or
// Delete, which 1.x had as functions and 2.0 has as constants.
func TestChangelogRemovedNamesAreNotContractNames(t *testing.T) {
	contract := contractSymbols(t, "CONTRACT.md")

	var live []string
	for name := range changelogRemovedNames(t) {
		if contract[name] {
			live = append(live, name)
		}
	}
	sort.Strings(live)
	if len(live) > 0 {
		t.Errorf("%s lists %d names as removed that CONTRACT.md declares:\n  %s",
			changelogPath, len(live), strings.Join(live, "\n  "))
	}
}

// alphaExportsAbsentFromContract is every name v1.2.0-alpha.1 exported at the
// top level that CONTRACT.md does not declare at the top level; a name 2.0
// keeps only as a method or field (Confirm, now Run.Confirm) counts as absent.
func alphaExportsAbsentFromContract(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(alphaExportsPath)
	if err != nil {
		t.Fatalf("read %s: %v", alphaExportsPath, err)
	}
	declared := map[string]bool{}
	for symbol := range contractSymbols(t, "CONTRACT.md") {
		top, _, _ := strings.Cut(symbol, ".")
		declared[top] = true
	}
	absent := map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		name := strings.TrimSpace(line)
		if name != "" && !strings.HasPrefix(name, "#") && !declared[name] {
			absent[name] = true
		}
	}
	if len(absent) == 0 {
		t.Fatalf("%s: no names absent from CONTRACT.md", alphaExportsPath)
	}
	return absent
}

// changelogRemovedNames is every name in a "- **Removed...**" bullet of
// CHANGELOG.md's pre-release section.
func changelogRemovedNames(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("read %s: %v", changelogPath, err)
	}
	names := map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		if !strings.HasPrefix(line, removedBulletStart) {
			continue
		}
		for _, m := range backtickedName.FindAllStringSubmatch(line, -1) {
			names[m[1]] = true
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s: no %q bullets found", changelogPath, removedBulletStart)
	}
	return names
}

// migrationGuideNames is every identifier in the first cell of a table row
// of the migration guide.
func migrationGuideNames(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(migrationGuidePath)
	if err != nil {
		t.Fatalf("read %s: %v", migrationGuidePath, err)
	}
	names := map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		firstCell, _, _ := strings.Cut(strings.TrimPrefix(line, "|"), "|")
		for _, id := range identifier.FindAllString(firstCell, -1) {
			names[id] = true
		}
	}
	return names
}
