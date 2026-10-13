package evo

import (
	"slices"
	"testing"
	"time"
)

// TestLiveTalliesAreClonedBeforeRender pins the call site that keeps a frame
// safe: the item tallies a frame is built from must not share backing with
// the live index, which keeps filing skipped items while a frame is read.
func TestLiveTalliesAreClonedBeforeRender(t *testing.T) {
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(&countingLiveSurface{}), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.close() })
	items := out.Group("items")
	items.Task("first").skipped(reason("pinned"))
	items.Task("second").skipped(reason("pinned"))

	out.mu.Lock()
	defer out.mu.Unlock()
	col := out.containerStates[items.id]
	const frameRows = 24
	_, frozen := col.settled().project(col, frameRows)
	want := slices.Clone(frozen.Items.Skipped.Reasons()[0].Names)

	out.mu.Unlock()
	items.Task("third").skipped(reason("pinned"))
	out.mu.Lock()
	_, _ = col.settled().project(col, frameRows)

	if got := frozen.Items.Skipped.Reasons()[0].Names; !slices.Equal(got, want) {
		t.Fatalf("a frame's item names changed after the live index filed another skipped item: got %v, want %v", got, want)
	}
	if len(want) != 2 {
		t.Fatalf("frame held %v skipped names, want the 2 filed before it was built", want)
	}
}
