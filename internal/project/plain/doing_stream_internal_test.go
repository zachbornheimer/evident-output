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

// TestPlainDoesNotStreamALinePerDoingCall pins CONTRACT §2: Task.Doing is
// live only. A loop naming its current item writes nothing durable per call
// (progress thins to milestones); the periodic heartbeat row carries the
// latest item and count for a Task that stays running.
func TestPlainDoesNotStreamALinePerDoingCall(t *testing.T) {
	const items = 1000
	newCopy := func() (*blockRun, *stepClock) {
		b, clk := newBlockRun(t), &stepClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		b.stream.cfg.Clock, b.stream.cfg.Lock = clk, &sync.Mutex{}
		b.task("copy", "")
		b.stream.Started("copy", clk.Now())
		return b, clk
	}
	itemName := func(i int) string { return fmt.Sprintf("f%04d", i) }

	t.Run("a bare Doing loop writes no line", func(t *testing.T) {
		b, _ := newCopy()
		rec := b.rows.Task("copy").Rec
		for i := range items {
			rec.SetPhase(itemName(i))
		}
		if got := b.written(); got != "" {
			t.Fatalf("%d Doing calls wrote %d lines, want none:\n%s", items, strings.Count(got, "\n"), got)
		}
	})

	t.Run("Progress with Doing thins to milestones", func(t *testing.T) {
		b, clk := newCopy()
		rec := b.rows.Task("copy").Rec
		for i := range items {
			rec.ApplyProgress(int64(i), items, record.Determinate)
			rec.SetPhase(itemName(i))
			b.stream.Progressed("copy", clk.Now())
		}
		if lines := strings.Count(b.written(), "\n"); lines > progressMilestones+1 {
			t.Fatalf("%d Progress+Doing calls wrote %d lines, want at most %d:\n%s", items, lines, progressMilestones+1, b.written())
		}
	})

	t.Run("the heartbeat shows the latest item and count", func(t *testing.T) {
		b, clk := newCopy()
		rec := b.rows.Task("copy").Rec
		for i := range items / 2 {
			rec.ApplyProgress(int64(i), items, record.Determinate)
			rec.SetPhase(itemName(i))
		}
		clk.advance(heartbeatInterval)

		want := fmt.Sprintf("%d/%d  %s", items/2-1, items, itemName(items/2-1))
		if got := b.written(); strings.Count(got, "\n") != 1 || !strings.Contains(got, want) || !strings.Contains(got, "— 30s") {
			t.Fatalf("want one heartbeat row naming %q and 30s, got:\n%s", want, got)
		}
	})
}
