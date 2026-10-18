package plain

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// milestoneRun is one Running Task "copy" narrating through a Plain stream.
type milestoneRun struct {
	*blockRun
	clock *stepClock
}

func newMilestoneRun(t *testing.T) milestoneRun {
	t.Helper()
	b, clk := newBlockRun(t), &stepClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	b.stream.cfg.Clock, b.stream.cfg.Lock = clk, &sync.Mutex{}
	b.task("copy", "")
	b.stream.Started("copy", clk.Now())
	return milestoneRun{blockRun: b, clock: clk}
}

// progress is Progress(completed, total): the count lands before any item.
func (m milestoneRun) progress(completed, total int) {
	m.rows.Task("copy").Rec.ApplyProgress(int64(completed), int64(total), record.Determinate)
	m.stream.Progressed("copy", m.clock.Now())
}

// doing is Doing(item).
func (m milestoneRun) doing(item string) {
	m.rows.Task("copy").Rec.SetPhase(item)
	m.stream.Narrated("copy", m.clock.Now())
}

func (m milestoneRun) lines() []string {
	return strings.Split(strings.TrimRight(m.written(), "\n"), "\n")
}

// TestPlainMilestonesAboutTenForEveryTotal pins CONTRACT §2 for Task.Progress
// in Plain: about ten milestone lines for any total, the first and the final
// count always, and each line after the first carries the Doing text current
// at its count: the item named by the Doing call that follows the tick
// (Progress(i, n).Doing(item i)), never the item of the call before it.
func TestPlainMilestonesAboutTenForEveryTotal(t *testing.T) {
	for _, total := range []int{3, 13, 19, 29, 111, 1000} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			m := newMilestoneRun(t)
			for i := 1; i <= total; i++ {
				m.progress(i, total)
				m.doing(fmt.Sprintf("item %d", i))
				m.clock.advance(10 * time.Millisecond)
			}
			lines := m.lines()
			if floor := min(total, progressMilestones-3); len(lines) < floor || len(lines) > progressMilestones+2 {
				t.Fatalf("total %d wrote %d lines, want %d to %d:\n%s", total, len(lines), floor, progressMilestones+2, m.written())
			}
			if last := lines[len(lines)-1]; !strings.Contains(last, fmt.Sprintf("%d/%d", total, total)) {
				t.Fatalf("total %d: the final line is %q, want the final count", total, last)
			}
			for _, line := range lines[1:] {
				var count, of int
				var item string
				if _, err := fmt.Sscanf(strings.TrimPrefix(line, "◐ copy"), "%d/%d  item %s", &count, &of, &item); err != nil || item != fmt.Sprint(count) {
					t.Fatalf("total %d: line %q must name the item current at its count (err %v):\n%s", total, line, err, m.written())
				}
			}
		})
	}

	t.Run("a Task that never narrates writes each milestone at once", func(t *testing.T) {
		m := newMilestoneRun(t)
		m.progress(1, 100)
		m.progress(10, 100)
		if lines := m.lines(); len(lines) != 2 || !strings.HasSuffix(lines[1], "10/100") {
			t.Fatalf("a tick with no Doing waited for one:\n%s", m.written())
		}
	})

	t.Run("a Task that stops pairing its ticks with Doing is not kept waiting", func(t *testing.T) {
		m := newMilestoneRun(t)
		for i := 1; i <= 25; i++ {
			m.progress(i, 100)
			m.doing(fmt.Sprintf("item %d", i))
		}
		before := len(m.lines())
		m.progress(50, 100)
		if len(m.lines()) != before {
			t.Fatalf("a paired Task's milestone did not wait for its Doing:\n%s", m.written())
		}
		m.progress(60, 100)
		m.progress(70, 100)
		if got := m.lines(); len(got) != before+2 || !strings.Contains(got[before], "60/100") || !strings.Contains(got[before+1], "70/100") {
			t.Fatalf("ticks after the pairing stopped did not write at once:\n%s", m.written())
		}
	})

	t.Run("the final tick is written when the Task settles without another Doing", func(t *testing.T) {
		m := newMilestoneRun(t)
		for i := 1; i <= 20; i++ {
			m.progress(i, 20)
			m.doing(fmt.Sprintf("item %d", i))
		}
		before := len(m.lines())
		m.progress(20, 20)
		m.rows.Task("copy").Rec.Settle(record.Succeeded)
		m.stream.Settled("copy")
		if got := m.written(); !strings.Contains(got[strings.LastIndex(got, "◐"):], "20/20") {
			t.Fatalf("settling dropped the owed final milestone (%d lines before):\n%s", before, got)
		}
	})
}
