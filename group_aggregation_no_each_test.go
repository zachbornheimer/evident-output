package evo_test

import (
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestGroup_LargePlainGroupAggregatesWithoutEach is the red-first proof that
// spec §25's "aggregation is renderer-owned and automatic ... does not
// require a special Each API" holds for an ordinary Group of explicitly
// declared children, now that the public Task.Each API is gone (7ec4d77
// removed it for 1.0.0). Before this test, only children internally marked
// FromEach got the minimal "attention rows only" aggregation
// (writeLiveEachAggregate/selectEachAttentionChildren); a plain
// group.Task(...) loop fell through to selectLiveChildren, which pads the
// row budget with routine Done rows whenever there is spare vertical room —
// so a 20-row terminal (room for ~18 child rows) showed one failing child,
// one running child, and roughly sixteen padding "✓ package-NNN" rows
// instead of the aggregated summary the spec's Group example promises.
func TestGroup_LargePlainGroupAggregatesWithoutEach(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.Height(20),
		testkit.NoColor(),
	)
	out := evo.Init(evo.Config{
		Stdout: io.Discard, Stderr: io.Discard, Isolated: true,
		Terminal: screen, VisibilityDelay: evo.DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.Close() })

	packages := out.Group("packages")
	for n := range 100 {
		task := packages.Task(fmt.Sprintf("package-%03d", n))
		switch n {
		case 7:
			task.Fail("checksum mismatch")
		case 42:
			task.Doing("downloading")
		default:
			task.Done()
		}
	}

	got := screen.LatestLiveText()
	if !strings.Contains(got, "package-007") || !strings.Contains(got, "checksum mismatch") {
		t.Fatalf("the failing child must surface:\n%s", got)
	}
	if !strings.Contains(got, "package-042") {
		t.Fatalf("the one Running activity child must surface:\n%s", got)
	}
	if !strings.Contains(got, "100") {
		t.Fatalf("the parent line must report the aggregate count out of 100:\n%s", got)
	}

	doneRows := strings.Count(got, "✓ package-")
	if doneRows > 0 {
		t.Fatalf("a large aggregated Group must not pad the row budget with routine Done children (found %d), got:\n%s", doneRows, got)
	}
}

// TestGroup_SmallGroupStillShowsEveryChild guards the other half of the
// same rule: a Group small enough to fit under the row budget renders every
// child individually (spec §18's "✓ launch agent / ✓ write plist / ✓
// register / ✓ start" example) — the aggregation fix above must not also
// start hiding routine Done children in the common small-group case.
func TestGroup_SmallGroupStillShowsEveryChild(t *testing.T) {
	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.Height(20),
		testkit.NoColor(),
	)
	out := evo.Init(evo.Config{
		Stdout: io.Discard, Stderr: io.Discard, Isolated: true,
		Terminal: screen, VisibilityDelay: evo.DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.Close() })

	agent := out.Group("launch agent")
	agent.Task("write plist").Done()
	agent.Task("register").Done()
	agent.Task("start").Doing("starting")

	got := screen.LatestLiveText()
	for _, name := range []string{"write plist", "register", "start"} {
		if !strings.Contains(got, name) {
			t.Fatalf("a small group must still show every child individually, missing %q:\n%s", name, got)
		}
	}
}
