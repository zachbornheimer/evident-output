package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// A Group whose collection turned out to be empty carries the answer on
// itself — `✓ branches  nothing to clean` — while its one explicit child
// carries the evidence (`✓ classify  146 tips`). Collapsing the group into
// that single child, which the one-child rule did unconditionally, deleted
// both the subject's name and the caller's Summary from the transcript: the
// reader saw a bare `✓ classify  146 tips` and never learned which subject
// had nothing to do (evo-rec spec, "nothing to do").
//
// The collapse is still right when the group has no Summary: then the child
// row says everything the header would have.
func TestGroup_SummaryKeepsTheGroupRowOverASingleExplicitChild(t *testing.T) {
	transcript := renderCleanSubject(t, "nothing to clean")

	if !strings.Contains(transcript, "branches") {
		t.Fatalf("group name is gone from the transcript:\n%s", transcript)
	}
	if !strings.Contains(transcript, "nothing to clean") {
		t.Fatalf("group Summary is gone from the transcript:\n%s", transcript)
	}
	if !strings.Contains(transcript, "146 tips") {
		t.Fatalf("explicit child is gone from the transcript:\n%s", transcript)
	}
}

func TestGroup_NoSummaryStillCollapsesIntoItsSingleExplicitChild(t *testing.T) {
	transcript := renderCleanSubject(t, "")

	if strings.Contains(transcript, "branches") {
		t.Fatalf("a summary-less group must not add a redundant header row:\n%s", transcript)
	}
	if !strings.Contains(transcript, "146 tips") {
		t.Fatalf("explicit child is gone from the transcript:\n%s", transcript)
	}
}

func renderCleanSubject(t *testing.T, summary string) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "zq", Isolated: true, Plain: true, Stdout: &buf, Stderr: &buf})
	group := out.Group("branches")
	group.Task("classify").Doing("classifying tips").Done("146 tips")
	if summary != "" {
		group.Summary(summary)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.String()
}
