package evo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// retiredCommentPhrase matches 1.x vocabulary a comment must not teach: the
// removed Output formats and options, and the retired tally, severity, Blocked
// and Wait wording. Each phrase is split with a non-capturing group so this
// file does not itself contain the text it forbids.
var retiredCommentPhrase = regexp.MustCompile(`\b(?:Format(?:JSONL?|Data|Human|External)|Also(?:Write)|Skipped/(?:Kept)|info (?:severity)|settles (?:Blocked)|tells a (?:Wait)|Wait (?:says)|taxonomy (?:tallies))\b`)

// retiredCommentPhrases finds the retired phrases one comment uses.
func retiredCommentPhrases(text string) []string {
	return retiredCommentPhrase.FindAllString(text, -1)
}

// TestCommentsTeachNoRetiredVocabulary: a comment in the root package or under
// internal never names a removed 1.x symbol as if it were live. The mcp module
// is left out because its rules quote retired names on purpose, and each such
// quote carries a "removed in" note.
func TestCommentsTeachNoRetiredVocabulary(t *testing.T) {
	for _, path := range goSourcesCitingContract(t) {
		if strings.HasPrefix(path, "mcp/") {
			continue
		}
		for _, c := range commentCitations(t, path, retiredCommentPhrases) {
			t.Errorf("%s teaches retired 1.x vocabulary %q; write the 2.0 spelling or delete the comment", c.pos, c.section)
		}
	}
}

// TestRetiredCommentPhraseFlagsAStaleComment proves the guard can fail.
func TestRetiredCommentPhraseFlagsAStaleComment(t *testing.T) {
	for _, stale := range []string{
		"Format" + "JSON's final document",
		"every Also" + "Write mirror",
		"expands each Skipped/" + "Kept row",
		"a Task settles " + "Blocked",
	} {
		if len(retiredCommentPhrases(stale)) == 0 {
			t.Errorf("%q was not flagged", stale)
		}
	}
	if got := retiredCommentPhrases("JSON Output writes the run document"); len(got) != 0 {
		t.Errorf("a 2.0 comment was flagged: %v", got)
	}
}

// sectionCitation matches a "§N" or "§N.M" citation; group 1 is the top-level section.
var sectionCitation = regexp.MustCompile(`§(\d+)(?:\.\d+)*`)

// contractSectionHeading matches a CONTRACT.md "## N. Title" heading.
var contractSectionHeading = regexp.MustCompile(`(?m)^## (\d+)\. `)

// citedSections finds the top-level section numbers a comment cites.
func citedSections(text string) []string {
	var sections []string
	for _, m := range sectionCitation.FindAllStringSubmatch(text, -1) {
		sections = append(sections, m[1])
	}
	return sections
}

// TestCommentsCiteOnlyContractSections: a root comment's §-citation names a
// section CONTRACT.md has, so a number from the retired spec cannot linger.
func TestCommentsCiteOnlyContractSections(t *testing.T) {
	contract, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatalf("read CONTRACT.md: %v", err)
	}
	have := map[string]bool{}
	for _, m := range contractSectionHeading.FindAllStringSubmatch(string(contract), -1) {
		have[m[1]] = true
	}
	if len(have) == 0 {
		t.Fatal("CONTRACT.md has no numbered sections")
	}
	roots, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob root Go files: %v", err)
	}
	for _, path := range roots {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		for _, c := range commentCitations(t, path, citedSections) {
			if !have[c.section] {
				t.Errorf("%s cites §%s, which CONTRACT.md does not have; cite a CONTRACT.md section or drop the number", c.pos, c.section)
			}
		}
	}
}
