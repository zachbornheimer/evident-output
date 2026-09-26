package evo_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDeferredTaskRowNotDoubledInSnapshotLines is the RED-then-GREEN
// regression for the release-gate probe: a standalone Task resolved while
// an earlier Println is held back behind a settled collection
// (commitResolvedTaskLocked's held branch) must render exactly once in
// evo.RenderPlain(out.Snapshot()) — as its own Task row — never a second
// time as a bare line pulled from Snapshot.Lines.
func TestDeferredTaskRowNotDoubledInSnapshotLines(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true,
		StateDir: t.TempDir(),
		Stdout:   &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("grp")
	child := group.Task("gchild")
	_ = child.Define(func(context.Context) error { return nil }).Wait()

	late := out.Task("late")
	out.Println("hello msg")
	_ = late.Define(func(context.Context) error { return nil }).Wait()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	for _, line := range snap.Lines {
		if strings.Contains(line, "late") {
			t.Fatalf("Task row leaked into Snapshot.Lines message log: %v", snap.Lines)
		}
	}

	rendered, err := evo.RenderPlain(snap, evo.PlainOptions{Width: 80, NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(rendered), "late"); got != 1 {
		t.Fatalf("want Task row \"late\" to render exactly once, got %d:\n%s", got, rendered)
	}
}
