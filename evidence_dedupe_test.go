package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFail_EvidenceDedupedAgainstSummary is red-first for P7's dedupe
// addition (user-13-problems.md Problem 7: "deduplicate it against the
// failure message"). The exact anti-pattern the doc names —
// a Fail whose summary embeds capture.Text() — folds the retained
// output straight into the summary; the auto-attached evidence tail must
// not then render the same text a second time underneath it.
func TestFail_EvidenceDedupedAgainstSummary(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	task := out.Task("install")
	output := task.EvidenceForTest()
	_, _ = fmt.Fprint(output, "npm ERR! 404 not found")
	task.Fail("install failed: " + output.Text())

	_ = out.Finish()

	rendered := buf.String()
	if got := strings.Count(rendered, "npm ERR! 404 not found"); got != 1 {
		t.Fatalf("want the retained output rendered exactly once (deduped against the summary that already contains it), got %d times in:\n%s", got, rendered)
	}
}

// TestFail_EvidenceStillRenders_WhenNotContainedInSummary proves the
// dedupe only skips a tail that IS already in the summary — genuinely new
// evidence (the paved-path Fail + auto-attach shape) still
// renders underneath.
func TestFail_EvidenceStillRenders_WhenNotContainedInSummary(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	task := out.Task("build")
	output := task.EvidenceForTest()
	_, _ = fmt.Fprintln(output, "error: undefined symbol foo")
	task.Fail("compile failed")

	_ = out.Finish()

	if rendered := buf.String(); !strings.Contains(rendered, "undefined symbol foo") {
		t.Fatalf("want the distinct evidence line still rendered, got:\n%s", rendered)
	}
}
