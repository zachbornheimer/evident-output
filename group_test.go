package evo

import (
	"io"
	"testing"
)

// TestGroup_ConcurrentIndependentChildrenAreNotMisuse pins the other side of
// the same contract: a plain DisplayGroup collection documents its children as
// independent (worker-pool fan-out), so two Running siblings there is a
// supported pattern, not misuse.
func TestGroup_ConcurrentIndependentChildrenAreNotMisuse(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	jobs := out.Group("dependencies")
	a := jobs.Task("discover")
	b := jobs.Task("verify")

	a.Doing("discovering")
	b.Doing("waiting") // second Running sibling — allowed on a plain DisplayGroup collection

	if err := out.err(); err != nil {
		t.Fatalf("want no misuse on an independent Tasks collection, got %v", err)
	}
}
