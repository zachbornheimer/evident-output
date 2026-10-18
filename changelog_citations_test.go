package evo

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// changelogUnreleasedHeading opens the CHANGELOG section for the release
// being prepared; it ends at the next "## " heading.
const changelogUnreleasedHeading = "## 2.0.0 (unreleased)"

// sectionSign matches a numbered-section citation: "§4", "§ 7", "§2.1".
var sectionSign = regexp.MustCompile(`§\s?(\d+(?:\.\d+)*)`)

// contractSectionWord matches "contract section 7" and "CONTRACT.md sections 4".
var contractSectionWord = regexp.MustCompile(`(?i)\bcontract(?:\.md)?\s+sections?\s+(\d+(?:\.\d+)*)`)

// changelogUnreleasedLines are the lines of the unreleased CHANGELOG section,
// with the 1-based line number of the first.
func changelogUnreleasedLines(t *testing.T) (lines []string, first int) {
	t.Helper()
	body, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	all := strings.Split(string(body), "\n")
	start := -1
	for i, line := range all {
		switch {
		case line == changelogUnreleasedHeading:
			start = i
		case start >= 0 && strings.HasPrefix(line, "## "):
			return all[start:i], start + 1
		}
	}
	if start < 0 {
		t.Fatalf("CHANGELOG.md has no %q section", changelogUnreleasedHeading)
	}
	return all[start:], start + 1
}

// TestChangelogCitesExistingContractSections: the unreleased CHANGELOG section
// describes the 2.0 contract, so every section it cites (§N, or "contract
// section N") is a section CONTRACT.md has. A 1.x-era citation of a high section
// number sends the reader to a longer document that no longer exists.
func TestChangelogCitesExistingContractSections(t *testing.T) {
	sections := contractSections(t)
	lines, first := changelogUnreleasedLines(t)
	for i, line := range lines {
		for _, pattern := range []*regexp.Regexp{sectionSign, contractSectionWord} {
			for _, m := range pattern.FindAllStringSubmatch(line, -1) {
				if !sections[m[1]] {
					t.Errorf("CHANGELOG.md:%d cites section %s; CONTRACT.md has %d sections:\n%s", first+i, m[1], len(sections), line)
				}
			}
		}
	}
}
