package evo_test

import (
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// liveBudgetHeight is a terminal short enough that a Group's header, its
// folded Skipped tally, and its running children cannot all fit.
const liveBudgetHeight = 8

// TestLiveGroup_AggregatedTalliesCountAgainstTheRowBudget holds the live
// frame to the terminal height once a Group's per-item Skipped children
// fold into a tally line (§25): that line is a row too, so the child rows
// the frame selects shrink to make room for it, and the omission line
// still accounts for what did not fit. Ordinary call sites moved to
// Skipped in 1.1 (§"Duplicate decisions"): this test used to hold two
// dispositions (Kept and Skipped) to the same budget; both reasons now
// fold into the one Skipped tally.
func TestLiveGroup_AggregatedTalliesCountAgainstTheRowBudget(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(liveBudgetHeight), testkit.NoColor())
	out := evo.Init(evo.Config{
		Isolated: true, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	packages := out.Group("packages")
	for n := range 40 {
		packages.Task(fmt.Sprintf("pinned-%02d", n)).Skipped(evo.Reason("pinned"))
		packages.Task(fmt.Sprintf("vendored-%02d", n)).Skipped(evo.Reason("vendored"))
	}
	for n := range 40 {
		packages.Task(fmt.Sprintf("package-%02d", n)).Doing("downloading")
	}

	frame := screen.LatestLiveText()
	for _, want := range []string{"- skipped 80 (40 pinned, 40 vendored)", "not shown"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("live frame lacks %q:\n%s", want, frame)
		}
	}
	if rows := strings.Count(frame, "\n") + 1; rows > liveBudgetHeight {
		t.Fatalf("live frame is %d rows, over the %d-row budget:\n%s", rows, liveBudgetHeight, frame)
	}
}
