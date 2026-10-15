package evo

import (
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
