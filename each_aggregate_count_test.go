package evo_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// itemNames builds n Each item names — the collection under test, not a
// fixture with meaning of its own.
func itemNames(prefix string, n int) []string {
	names := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		names = append(names, fmt.Sprintf("%s%d", prefix, i))
	}
	return names
}

// TestRender_P8_DurableEachAggregateShowsCount pins the P8/axis-7 gap: live
// counts the collection (`⠇ fix tools  0/5` … `✓ fix tools  5/5`) but the
// durable row printed a bare `✓ fix tools`, so at 1,000 children a reader
// could not tell 999/1000 from 1/1000 without the JSON.
func TestRender_P8_DurableEachAggregateShowsCount(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)

	for _, task := range out.Group("fix tools").Each(itemNames("tool", 5)) {
		task.Define(func() error { return nil })
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := collapse(buf.String()); !strings.Contains(got, "✓ fix tools 5/5") {
		t.Fatalf("want the durable aggregate count, got:\n%s", buf.String())
	}
}

// TestRender_P7_PartialEachAggregateCountsAndNamesTheUnstarted pins the
// P7/axis-9 gap: a collection that stopped early rendered its one failure
// and silently dropped every NotStarted sibling — no names, no count. The
// count says how far the run got; one derived line says how much never
// started.
func TestRender_P7_PartialEachAggregateCountsAndNamesTheUnstarted(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)

	const failAt = "pkg4"
	for name, task := range out.Sequence("install").Each(itemNames("pkg", 10)) {
		task.Define(func() error {
			if name == failAt {
				return errProbe
			}
			return nil
		})
	}
	_ = out.Finish()

	got := collapse(buf.String())
	if !strings.Contains(got, "✗ install 3/10") {
		t.Fatalf("want the partial aggregate count, got:\n%s", buf.String())
	}
	if !strings.Contains(got, "- 6 not started") {
		t.Fatalf("want the derived not-started count, got:\n%s", buf.String())
	}
	if strings.Contains(got, "pkg7") {
		t.Fatalf("not-started children are counted, never named, got:\n%s", buf.String())
	}
}

// TestRender_AggregateCountsEachChildrenOnly pins the count's denominator:
// a Group's completed/total is derived from its Each-created atomic
// children. An explicitly declared child stays individually visible on its
// own row and must not inflate the collection it sits beside — the live
// evidence was `✓ branches  146/146` over a 145-branch repo.
func TestRender_AggregateCountsEachChildrenOnly(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)

	group := out.Group("branches")
	for _, task := range group.Each(itemNames("branch", 3)) {
		task.Define(func() error { return nil })
	}
	group.Task("classify tips").Define(func() error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got := collapse(buf.String())
	if !strings.Contains(got, "✓ branches 3/3") {
		t.Fatalf("want the Each-only denominator, got:\n%s", buf.String())
	}
	if strings.Contains(got, "4/4") {
		t.Fatalf("an explicit child must not enter the count, got:\n%s", buf.String())
	}
}

// TestLive_AggregateCountsEachChildrenOnly is the live half of the same
// contract: both surfaces derive the count from the same children, so the
// live frame and the durable row can never disagree.
func TestLive_AggregateCountsEachChildrenOnly(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.Init(evo.Config{
		Stdout: io.Discard, Stderr: io.Discard, Isolated: true,
		Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever,
	})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("branches")
	for _, task := range group.Each(itemNames("branch", 3)) {
		task.Skipped(evo.Reason("protected"))
	}
	group.Task("classify tips").Doing("classifying tips")

	if got := collapse(screen.LatestLiveText()); !strings.Contains(got, "branches 3/3") {
		t.Fatalf("want the Each-only denominator live, got:\n%s", screen.LatestLiveText())
	}
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
