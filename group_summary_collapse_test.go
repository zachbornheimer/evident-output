package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// A group's header row carries two things its child rows cannot: the
// group's own name, and the caller's Summary. The one-child collapse fired
// on count alone, so a subject whose collection turned out empty —
// `✓ branches  nothing to clean` over one `✓ classify  146 tips` child —
// rendered as a bare `✓ classify  146 tips`, and the reader never learned
// which subject had nothing to do (evo-rec spec, "nothing to do").
func TestGroup_SummaryKeepsTheGroupRowOverASingleExplicitChild(t *testing.T) {
	transcript := renderSubject(t, "branches", "classify", "nothing to clean")

	for _, want := range []string{"branches", "nothing to clean", "146 tips"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("%q is gone from the transcript:\n%s", want, transcript)
		}
	}
}

// The same lossy collapse hid the subject name on every failure path: three
// sibling subjects that all failed before declaring any collection rendered
// as three identical `classify` rows with no way to tell them apart.
func TestGroup_DifferentlyNamedChildKeepsTheGroupRow(t *testing.T) {
	transcript := renderSubject(t, "branches", "classify", "")

	if !strings.Contains(transcript, "branches") {
		t.Fatalf("group name is gone from the transcript:\n%s", transcript)
	}
	if !strings.Contains(transcript, "146 tips") {
		t.Fatalf("explicit child is gone from the transcript:\n%s", transcript)
	}
}

// A header that would merely repeat its only child is still redundant.
func TestGroup_ChildRepeatingTheGroupNameStillCollapses(t *testing.T) {
	transcript := renderSubject(t, "branches", "branches", "")

	if strings.Count(transcript, "branches") != 1 {
		t.Fatalf("a header repeating its only child must not render twice:\n%s", transcript)
	}
}

func renderSubject(t *testing.T, groupName, childName, summary string) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "zq", Isolated: true, Plain: true, Stdout: &buf, Stderr: &buf})
	group := out.Group(groupName)
	group.Task(childName).Doing("classifying tips").Done("146 tips")
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
