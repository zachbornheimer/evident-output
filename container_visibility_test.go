package evo

import (
	"bytes"
	"strings"
	"testing"
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
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	g := out.Group("same")
	succeed(g.Task("same"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	lines := nonEmptyLines(buf.String())
	if n := countLinesContaining(lines, "same"); n != 1 {
		t.Fatalf("collapsed Group should render exactly one row naming %q, got %d in:\n%s", "same", n, buf.String())
	}
}

// TestContainerVisibility_KeepsHeaderOfGroupWithoutOwnInformation proves the
// container-visibility decision is independent of whether the child is a
// Task: collapse folds routine detail but never a container's own row
// (contract §8), so a Group with no Summary keeps its header and the
// nested Group's content renders indented beneath it.
func TestContainerVisibility_KeepsHeaderOfGroupWithoutOwnInformation(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	outer := out.Group("outer")
	inner := outer.Group("inner")
	succeed(inner.Task("inner"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	lines := nonEmptyLines(buf.String())
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "✓ outer") || !strings.HasPrefix(lines[1], "   ✓ inner") {
		t.Fatalf("want the outer row with the inner row indented beneath it, got:\n%s", buf.String())
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
