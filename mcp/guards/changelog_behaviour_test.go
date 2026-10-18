package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// renderedLine matches a code span that quotes the two shapes CONTRACT.md
// section 4 pins letter for letter: the Cancelled band and the "!" finding
// line under a Failed or Cancelled band.
var renderedLine = regexp.MustCompile("`(\\[cancelled\\][^`\n]*|![^`\n]*)`")

// notExportedClaim marks a sentence that says a name cannot be referenced.
var notExportedClaim = regexp.MustCompile(`(?i)\b(?:no longer exported|not exported|unexported|not a name a caller can reference)\b`)

// TestChangelogCurrentSectionQuotesContractRows: the Cancelled band and "!"
// line the 2.0.0 section quotes in a code span appear in CONTRACT.md letter for
// letter, spaces included, so the release note cannot describe a different
// screen than the contract promises.
func TestChangelogCurrentSectionQuotesContractRows(t *testing.T) {
	root := moduleRoot(t)
	contract, err := os.ReadFile(filepath.Join(root, "CONTRACT.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines, first := currentChangelogSection(t, filepath.Join(root, "CHANGELOG.md"))
	for i, line := range lines {
		for _, m := range renderedLine.FindAllStringSubmatch(line, -1) {
			quoted := strings.TrimSuffix(strings.TrimSuffix(m[1], "..."), " ")
			if !strings.Contains(string(contract), quoted) {
				t.Errorf("CHANGELOG.md:%d quotes %q, which CONTRACT.md does not contain:\n%s", first+i, quoted, line)
			}
		}
	}
}

// TestChangelogCurrentSectionDoesNotHideContractNames: a sentence of the
// 2.0.0 section that says a name is not exported names no symbol CONTRACT.md
// lists.
func TestChangelogCurrentSectionDoesNotHideContractNames(t *testing.T) {
	root := moduleRoot(t)
	surface := readContractSurface(t, filepath.Join(root, "CONTRACT.md"))
	lines, first := currentChangelogSection(t, filepath.Join(root, "CHANGELOG.md"))
	for i, line := range lines {
		if !notExportedClaim.MatchString(line) {
			continue
		}
		reported := map[string]bool{}
		for _, claim := range notExportedClaim.FindAllStringIndex(line, -1) {
			sentence, _ := sentenceAround(line, claim[0])
			if reported[sentence] {
				continue
			}
			reported[sentence] = true
			for _, span := range changelogCodeSpan.FindAllStringSubmatch(sentence, -1) {
				if name := strings.TrimPrefix(span[1], "evo."); surface.names[name] {
					t.Errorf("CHANGELOG.md:%d says %s is not exported; CONTRACT.md lists it:\n%s", first+i, name, line)
				}
			}
		}
	}
}
