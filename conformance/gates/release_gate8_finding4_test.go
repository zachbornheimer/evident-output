package gates_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true, Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("install")
	packages := []string{"alpha", "bravo", "charlie"}
	for pkg, task := range g.Each(packages) {
		if pkg == "bravo" {
			task.Fail("install bravo")
			break
		}
		task.Define(func() error { return nil })
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "✗") || !strings.Contains(got, "bravo") || !strings.Contains(got, "install bravo") {
		t.Fatalf("want the failed Each child to surface under the aggregate, got:\n%s", got)
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
		Terminal: screen, VisibilityDelay: new(time.Duration), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("install")
	packages := []string{"alpha", "bravo", "charlie"}
	for pkg, task := range g.Each(packages) {
		if pkg == "bravo" {
			task.Fail("install bravo")
			break
		}
		task.Define(func() error { return nil })
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	found := false
	for _, op := range screen.Operations() {
		if strings.Contains(op.Text, "install") && strings.Contains(op.Text, "/") && strings.Contains(op.Text, "bravo") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want a live aggregate count plus the failed child, got:\n%#v", screen.Operations())
	}
}
