package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// changelogCurrentHeading opens the section for the release being prepared.
const changelogCurrentHeading = "## 2.0.0 (unreleased)"

// changelogRemovedHeading opens the subsection that lists what the release
// removed; names there are the point of the list.
const changelogRemovedHeading = "### Removed"

// changelogCodeSpan matches an inline code span.
var changelogCodeSpan = regexp.MustCompile("`([^`\n]+)`")

// changelogName matches one Go identifier, or evo.X / Type.Member, inside a
// code span, up to a call or type parameter.
var changelogName = regexp.MustCompile(`^(?:(evo)\.)?([A-Z]\w*)(?:\.([A-Z]\w*))?(?:[(\[].*)?$`)

// currentChangelogSection is the 2.0.0 section of CHANGELOG.md: its heading
// to the next "## " heading, as lines with their 1-based line numbers.
func currentChangelogSection(t *testing.T, path string) (lines []string, firstLine int) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	all := strings.Split(string(body), "\n")
	start := -1
	for i, line := range all {
		switch {
		case line == changelogCurrentHeading:
			start = i
		case start >= 0 && strings.HasPrefix(line, "## "):
			return all[start:i], start + 1
		}
	}
	if start < 0 {
		t.Fatalf("%s has no %q section", path, changelogCurrentHeading)
	}
	return all[start:], start + 1
}

// removedListLines are the indexes of lines that belong to a list of removed
// names: the "### Removed" subsection, and any bullet that opens with
// "**Removed".
func removedListLines(lines []string) map[int]bool {
	inList := map[int]bool{}
	inSubsection, inBullet := false, false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "### "):
			inSubsection = line == changelogRemovedHeading
			inBullet = false
		case strings.HasPrefix(line, "- "):
			inBullet = strings.HasPrefix(line, "- **Removed")
		case strings.TrimSpace(line) == "":
			inBullet = false
		}
		inList[i] = inSubsection || inBullet
	}
	return inList
}

// TestChangelogCurrentSectionNamesOnlyContractSymbols fails when the 2.0.0
// section of CHANGELOG.md names, in a code span, an evo.X or Type.Member that
// CONTRACT.md does not list, or a bare name its own lists of removed names
// retired, anywhere but in those lists. A sentence may name a symbol
// to say it was removed. Entries that describe an intermediate release's API
// belong in that release's own section.
func TestChangelogCurrentSectionNamesOnlyContractSymbols(t *testing.T) {
	root := moduleRoot(t)
	surface := readContractSurface(t, filepath.Join(root, "CONTRACT.md"))
	lines, first := currentChangelogSection(t, filepath.Join(root, "CHANGELOG.md"))
	inRemovedList := removedListLines(lines)
	retired := removedNames(lines, inRemovedList, surface)
	for i, line := range lines {
		// Main takes the Config first; the 1.x spelling is wrong even where
		// the section lists what 2.0 removed.
		if oneArgMain.MatchString(line) {
			t.Errorf("CHANGELOG.md:%d spells evo.Main(run); 2.0 is evo.Main(cfg, run):\n%s", first+i, line)
		}
		if inRemovedList[i] || saysRemoved.MatchString(line) {
			continue
		}
		for _, span := range changelogCodeSpan.FindAllStringSubmatch(line, -1) {
			if bad := changelogViolation(surface, retired, span[1]); bad != "" {
				t.Errorf("CHANGELOG.md:%d names %s, which CONTRACT.md does not list:\n%s", first+i, bad, line)
			}
		}
		for _, hit := range unqualifiedRetiredNames(line) {
			t.Errorf("CHANGELOG.md:%d uses the retired name %s outside a list of removed names:\n%s", first+i, hit.name, line)
		}
	}
}

// oneArgMain matches the 1.x call evo.Main(run), which 2.0 takes a Config for.
var oneArgMain = regexp.MustCompile(`evo\.Main\(run\)`)

// saysRemoved marks a line that says a name was removed.
var saysRemoved = regexp.MustCompile(`(?i)\bremoved\b`)

// exportedIdentifier matches one exported Go identifier.
var exportedIdentifier = regexp.MustCompile(`[A-Z]\w*`)

// removedNames are the identifiers the section's lists of removed names put
// in code spans, less every name CONTRACT.md lists (Create, Update and Delete
// were 1.x functions and are 2.0 constants).
func removedNames(lines []string, inRemovedList map[int]bool, surface contractSurface) map[string]bool {
	names := map[string]bool{}
	for i, line := range lines {
		if !inRemovedList[i] {
			continue
		}
		for _, span := range changelogCodeSpan.FindAllStringSubmatch(line, -1) {
			for _, id := range exportedIdentifier.FindAllString(span[1], -1) {
				if !surface.knows(id) {
					names[id] = true
				}
			}
		}
	}
	return names
}

// changelogViolation is the name in span the contract does not list, or "".
func changelogViolation(surface contractSurface, retired map[string]bool, span string) string {
	m := changelogName.FindStringSubmatch(span)
	if m == nil {
		return ""
	}
	qualifier, first, member := m[1], m[2], m[3]
	switch {
	case qualifier == "evo" && member == "":
		if !surface.names[first] {
			return "evo." + first
		}
	case qualifier == "" && member != "":
		if _, typed := surface.members[first]; typed && !surface.members[first][member] {
			return first + "." + member
		}
	case qualifier == "" && retired[first] && !surface.knows(first):
		return first
	}
	return ""
}
