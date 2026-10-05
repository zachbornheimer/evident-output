package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestGroup_ConcurrentIndependentChildrenAreNotMisuse pins the other side of
// the same contract: a plain DisplayGroup collection documents its children as
// independent (worker-pool fan-out), so two Running siblings there is a
// supported pattern, not misuse.
func TestGroup_ConcurrentIndependentChildrenAreNotMisuse(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.Group("dependencies")
	a := jobs.Task("discover")
	b := jobs.Task("verify")

	a.Doing("discovering")
	b.Doing("waiting") // second Running sibling — allowed on a plain DisplayGroup collection

	if err := out.Err(); err != nil {
		t.Fatalf("want no misuse on an independent Tasks collection, got %v", err)
	}
}
