package evo_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestLive_PerItemGroupsKeepHeaderAndAttention pins E-111: a Group of more
// per-item Groups than the frame has rows gave each nested Group a share
// of 0 rows, so each painted a bare "…  1 not shown" in place of its row,
// the outer header vanished, and the finished frame was 23 identical
// omission lines the repaint dedup then froze. The outer header now stays
// with its N/M count, the unfinished items fill the rows in attention
// order, and the finished ones fold into one omission line.
func TestLive_PerItemGroupsKeepHeaderAndAttention(t *testing.T) {
	const n, workers = 30, 4
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.Init(evo.Config{
		Isolated: true, Clock: testkit.NewClock(), Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000, MaxConcurrency: workers,
	})
	t.Cleanup(func() { _ = out.Close() })

	items := out.Group("items")
	release, started := make([]chan struct{}, n), make([]chan struct{}, n)
	tasks := make([]*evo.TaskHandle, n)
	for i := range n {
		release[i], started[i] = make(chan struct{}), make(chan struct{})
		tasks[i] = items.Group(fmt.Sprintf("item %d", i)).Task("a").Define(func(context.Context) error {
			close(started[i])
			<-release[i]
			return nil
		})
	}
	const finished = 10
	for i := range finished {
		<-started[i]
		close(release[i])
		_ = tasks[i].Wait()
	}
	for i := finished; i < finished+workers; i++ {
		<-started[i]
	}

	mid := screen.LatestLiveText()
	glyph := spinnerOf(mid, fmt.Sprintf("item %d ", finished))
	var want strings.Builder
	fmt.Fprintf(&want, "%s items  %d/%d complete\n", glyph, finished, n)
	for i := finished; i < finished+workers; i++ {
		fmt.Fprintf(&want, "   %s item %d  a  working…\n", glyph, i)
	}
	for i := finished + workers; i < n; i++ {
		fmt.Fprintf(&want, "   ○ item %d  a\n", i)
	}
	fmt.Fprintf(&want, "   …  %d not shown", finished)
	if mid != want.String() {
		t.Errorf("mid-run frame:\n--- want ---\n%s\n--- got ---\n%s", want.String(), mid)
	}

	for i := finished; i < n; i++ {
		<-started[i]
		close(release[i])
		_ = tasks[i].Wait()
	}
	_ = items.Wait()
	if got, want := screen.LatestLiveText(), fmt.Sprintf("✓ items\n   …  %d not shown", n); got != want {
		t.Errorf("finished frame:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// spinnerOf is the glyph that starts the first row of frame naming row.
func spinnerOf(frame, row string) string {
	for line := range strings.SplitSeq(frame, "\n") {
		if strings.Contains(line, row) {
			return firstRune(strings.TrimLeft(line, " "))
		}
	}
	return ""
}
