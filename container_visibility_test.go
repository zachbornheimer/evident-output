package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// ZYS-838 acceptance: "Renderer tests prove a container may be visible or
// collapsed independently of whether its children are Tasks." Group/
// Sequence organize work; they are not fake Tasks reached for merely to
// earn a row, and whether their own header row survives is a renderer
// decision distinct from what kind of children they hold.

// TestContainerVisibility_CollapsesLoneSameNameTask is the baseline: a
// Group whose entire content is one Task sharing the Group's own name, and
// no nested container, collapses into that Task's single row.
func TestContainerVisibility_CollapsesLoneSameNameTask(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	g := out.Group("same")
	g.Task("same").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	lines := nonEmptyLines(buf.String())
	if n := countLinesContaining(lines, "same"); n != 1 {
		t.Fatalf("collapsed Group should render exactly one row naming %q, got %d in:\n%s", "same", n, buf.String())
	}
}

// TestContainerVisibility_HidesHeaderOfGroupWithoutOwnInformation proves the
// container-visibility decision is independent of whether the child is a
// Task: a Group with no Summary of its own has nothing to say beyond its
// children, whatever kind they are, so its header row is not rendered
// (contract §3/§18) and the nested Group's content still shows.
func TestContainerVisibility_HidesHeaderOfGroupWithoutOwnInformation(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	outer := out.Group("outer")
	inner := outer.Group("inner")
	inner.Task("inner").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Contains(s, "outer") {
		t.Fatalf("outer Group has no information of its own, so no header row:\n%s", s)
	}
	if !strings.Contains(s, "inner") {
		t.Fatalf("nested inner Group's content must still render:\n%s", s)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func countLinesContaining(lines []string, substr string) int {
	n := 0
	for _, line := range lines {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}
