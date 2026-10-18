package plain

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// stepClock is a scheduling clock whose time and timers move only when the
// test advances it.
type stepClock struct {
	now    time.Time
	timers []func()
}

func (c *stepClock) Now() time.Time { return c.now }
func (c *stepClock) AfterFunc(_ time.Duration, fn func()) func() {
	c.timers = append(c.timers, fn)
	return func() {}
}

// advance moves time forward and fires every armed check, as the real
// scheduler would once its wait has passed.
func (c *stepClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
	due := c.timers
	c.timers = nil
	for _, fire := range due {
		fire()
	}
}

// TestPlainDoingFirstCallIsDurableThenMilestones pins CONTRACT §2: in Plain
// the first call of Doing is a durable line at once, and later calls thin
// into milestones that carry the latest text. A loop naming a thousand items
// never writes a line per name; Progress with Doing stays at its own
// milestones; the heartbeat carries the latest item and count after them.
func TestPlainDoingFirstCallIsDurableThenMilestones(t *testing.T) {
	const items = 1000
	newCopy := func() (*blockRun, *stepClock) {
		b, clk := newBlockRun(t), &stepClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		b.stream.cfg.Clock, b.stream.cfg.Lock = clk, &sync.Mutex{}
		b.task("copy", "")
		b.stream.Started("copy", clk.Now())
		return b, clk
	}
	itemName := func(i int) string { return fmt.Sprintf("f%04d", i) }
	narrate := func(b *blockRun, clk *stepClock, i int) {
		b.rows.Task("copy").Rec.SetPhase(itemName(i))
		b.stream.Narrated("copy", clk.Now())
	}

	t.Run("the first call is a line at once", func(t *testing.T) {
		b, clk := newCopy()
		narrate(b, clk, 0)
		if got := b.written(); strings.Count(got, "\n") != 1 || !strings.Contains(got, itemName(0)) {
			t.Fatalf("first Doing call wrote:\n%s\nwant one line naming %s", got, itemName(0))
		}
	})

	t.Run("a thousand calls thin into a handful of milestones", func(t *testing.T) {
		b, clk := newCopy()
		for i := range items {
			narrate(b, clk, i)
			clk.advance(10 * time.Millisecond)
		}
		lines := strings.Count(b.written(), "\n")
		if lines < 2 || lines > progressMilestones+2 {
			t.Fatalf("%d Doing calls wrote %d lines, want 2 to %d:\n%s", items, lines, progressMilestones+2, b.written())
		}
		if strings.Contains(b.written(), itemName(1)+"\n") {
			t.Fatalf("a call %v after the first wrote its own line:\n%s", 10*time.Millisecond, b.written())
		}
	})

	t.Run("a later milestone carries the latest text", func(t *testing.T) {
		b, clk := newCopy()
		narrate(b, clk, 0)
		clk.now = clk.now.Add(firstNarrationGap)
		narrate(b, clk, 1)
		if got := b.written(); !strings.HasSuffix(got, itemName(1)+"\n") {
			t.Fatalf("milestone after %v should end with %s, got:\n%s", firstNarrationGap, itemName(1), got)
		}
	})

	t.Run("Progress with Doing thins to the Progress milestones", func(t *testing.T) {
		b, clk := newCopy()
		rec := b.rows.Task("copy").Rec
		for i := range items {
			rec.ApplyProgress(int64(i), items, record.Determinate)
			narrate(b, clk, i)
			b.stream.Progressed("copy", clk.Now())
			clk.now = clk.now.Add(time.Second)
		}
		if lines := strings.Count(b.written(), "\n"); lines > progressMilestones+2 {
			t.Fatalf("%d Progress+Doing calls wrote %d lines, want at most %d:\n%s", items, lines, progressMilestones+2, b.written())
		}
	})

	t.Run("the heartbeat shows the latest item and count", func(t *testing.T) {
		b, clk := newCopy()
		rec := b.rows.Task("copy").Rec
		for i := range items / 2 {
			rec.ApplyProgress(int64(i), items, record.Determinate)
			narrate(b, clk, i)
		}
		first := strings.Count(b.written(), "\n")
		clk.advance(heartbeatInterval)

		want := fmt.Sprintf("%d/%d  %s", items/2-1, items, itemName(items/2-1))
		got := b.written()
		if strings.Count(got, "\n") != first+1 || !strings.HasSuffix(strings.TrimSpace(got), "— 30s") || !strings.Contains(got, want) {
			t.Fatalf("want one heartbeat row naming %q and 30s after %d lines, got:\n%s", want, first, got)
		}
	})
}
