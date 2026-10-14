package evo

import (
	"bytes"
	"strings"
	"testing"
)

// A NotStarted child is normally not a source of incompleteness: an earlier
// sibling in the same group already failed, and that sibling carries the
// group's verdict. When EVERY child is NotStarted there is no such sibling —
// the thing that stopped the run was a different subject entirely — so the
// group folded to Done and a subject that never ran rendered `✓ remotes`
// over its own `- classify  not started` row.
func TestGroup_EveryChildNotStartedIsNotStartedNotDone(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "clean", Output: Plain, Facades: Facades{Stdout: &buf, Stderr: &buf}})

	failing := out.Group("branches")
	failing.Task("classify").fail("not a git work tree")

	untouched := out.Group("remotes")
	untouched.Task("classify")

	_ = out.finish()
	_ = out.close()

	if got := untouched.snapshot().State; got != notStarted {
		t.Fatalf("group state = %v, want NotStarted; transcript:\n%s", got, buf.String())
	}
	if strings.Contains(buf.String(), "✓ remotes") {
		t.Fatalf("a subject that never ran must not render a check:\n%s", buf.String())
	}
}
