package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPlain_ColumnWidthIgnoresCollapsedEachChildren is the red-first proof
// for the canary's `✓ classify` + 40-80 spaces + `111 worktrees` row: the
// durable child column padded every rendered row out to the longest Each
// child name, including the hundred-odd children that collapsed into the
// aggregate and were never rendered at all. A column exists to align rows
// the reader can see; an invisible row cannot widen it.
func TestPlain_ColumnWidthIgnoresCollapsedEachChildren(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "zq", Stdout: &buf, Plain: true, Color: evo.ColorNever})

	worktrees := out.Group("worktrees")
	paths := make([]string, 111)
	for i := range paths {
		paths[i] = fmt.Sprintf("/Users/zach/Developer/Personal/.worktrees/agent-%016x", i)
	}
	for _, task := range worktrees.Each(paths) {
		task.Done()
	}
	worktrees.Task("classify").Done("111 worktrees")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := buf.String()
	var row string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "111 worktrees") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("no classify row in:\n%s", got)
	}
	if strings.Contains(row, strings.Repeat(" ", 10)) {
		t.Fatalf("classify row padded to an unrendered child's width:\n%q\nfull:\n%s", row, got)
	}
}
