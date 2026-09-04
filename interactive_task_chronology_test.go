package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestInteractive_ResolvedTaskCommitsBeforeLaterPrintln is release-gate
// round 5 finding 3: progressive.go's residualPlainLocked doc comment
// already states the contract ("interleave by completion/call time"), but
// the interactive path did not honor it for standalone tasks — a resolved
// Task row sat pinned in the live ticker until Finish (H.17) while a later
// Println streamed durably right away, so scrollback showed the Println
// line ABOVE a task that had actually resolved first. Chronology must be
// row-then-line: once a task resolves, its row commits to durable scrollback
// immediately, before any later Println's own durable write.
func TestInteractive_ResolvedTaskCommitsBeforeLaterPrintln(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("working tree").Done()
	out.Println("Dry-run: no changes will be made.")
	_ = out.Finish()

	var durable strings.Builder
	for _, op := range screen.Operations() {
		if op.Kind == "durable" {
			durable.WriteString(op.Text)
		}
	}
	got := durable.String()
	taskIdx := strings.Index(got, "working tree")
	lineIdx := strings.Index(got, "Dry-run")
	if taskIdx < 0 {
		t.Fatalf("resolved task row never committed to durable scrollback:\n%s", got)
	}
	if lineIdx < 0 {
		t.Fatalf("Println line never committed to durable scrollback:\n%s", got)
	}
	if taskIdx > lineIdx {
		t.Fatalf("chronology inverted: resolved task row must precede a later Println, got:\n%s", got)
	}
}
