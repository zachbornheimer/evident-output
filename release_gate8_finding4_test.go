package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestFailedTask_WithProgressRendersCountOnFailureRow is release-gate round
// 8 finding 4: a task that fails partway through an Each/Progress loop must
// keep its in-flight count visible on the failure row, in the same position
// a Running row shows it (glyph, name, count, message) — dropping the count
// the instant a task fails would hide exactly the evidence a reader needs
// most ("how far did it get before breaking").
func TestFailedTask_WithProgressRendersCountOnFailureRow(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("install")
	packages := []string{"alpha", "bravo", "charlie"}
	for pkg := range task.Each(packages) {
		if pkg == "bravo" {
			task.Fail("install bravo")
			break
		}
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "✗  install  1/3  install bravo") {
		t.Fatalf("want the failure row to carry the in-flight count in the live row's position, got:\n%s", got)
	}
}

// TestFailedTask_LiveRowRendersCountAtSamePosition is the interactive
// counterpart: the live spinner region's failure row places the count in
// the same position as its own Running row.
func TestFailedTask_LiveRowRendersCountAtSamePosition(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.NoColor(),
	)
	out := evo.Init(evo.Config{
		Isolated: true,
		Options: []evo.Option{
			evo.Terminal(screen),
			evo.VisibilityDelay(0),
			evo.NoColor(),
		},
	})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("install")
	packages := []string{"alpha", "bravo", "charlie"}
	for pkg := range task.Each(packages) {
		if pkg == "bravo" {
			task.Fail("install bravo")
			break
		}
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	found := false
	for _, op := range screen.Operations() {
		if strings.Contains(op.Text, "install") && strings.Contains(op.Text, "1/3") && strings.Contains(op.Text, "install bravo") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want a live/durable operation with the failure row's in-flight count, got:\n%#v", screen.Operations())
	}
}
