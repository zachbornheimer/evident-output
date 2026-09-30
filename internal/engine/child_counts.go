package engine

import (
	"container/heap"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// childCounters counts a set of child Tasks by what a live tally reports
// of them, and takes a Task back out when it moves to another set.
type childCounters struct {
	total, done, running, pending, unfinished int
	// widths counts the Tasks at each name width, in runes, so the widest
	// survives a removal.
	widths   map[int]int
	maxWidth int
}

func (c *childCounters) add(f *filing) {
	c.move(f, 1)
	if c.widths == nil {
		c.widths = make(map[int]int)
	}
	c.widths[f.width]++
	c.maxWidth = max(c.maxWidth, f.width)
}

func (c *childCounters) remove(f *filing) {
	c.move(f, -1)
	if c.widths[f.width]--; c.widths[f.width] > 0 {
		return
	}
	delete(c.widths, f.width)
	if f.width < c.maxWidth {
		return
	}
	c.maxWidth = 0
	for w := range c.widths {
		c.maxWidth = max(c.maxWidth, w)
	}
}

// move counts f's Task by delta in every per-state total.
func (c *childCounters) move(f *filing, delta int) {
	c.total += delta
	switch f.state {
	case Done, Skipped:
		c.done += delta
	case Running:
		c.running += delta
	case Pending:
		c.pending += delta
	}
	if !core.IsTerminalTask(f.state) {
		c.unfinished += delta
	}
}

// counts is c as a live tally, earliest being the earliest first paint
// among the counted Tasks.
func (c *childCounters) counts(earliest time.Time) core.ChildCounts {
	return core.ChildCounts{
		Total: c.total, Done: c.done,
		Running: c.running > 0, Pending: c.pending > 0, Unfinished: c.unfinished > 0,
		EarliestSeen: earliest, NameWidth: c.maxWidth,
	}
}

// stateCounts counts a collection's direct Tasks by the states the
// collection's verdict folds (verdictFold).
type stateCounts struct {
	running, failed, blocked, cancelled, notStarted, unresolved, warned int
}

func (s *stateCounts) add(f *filing)    { s.move(f, 1) }
func (s *stateCounts) remove(f *filing) { s.move(f, -1) }

func (s *stateCounts) move(f *filing, delta int) {
	switch f.state {
	case Running:
		s.running += delta
	case Failed:
		s.failed += delta
	case Blocked:
		s.blocked += delta
	case Cancelled:
		s.cancelled += delta
	case NotStarted:
		s.notStarted += delta
	case Done, Skipped:
	default:
		s.unresolved += delta
	}
	if f.warned {
		s.warned += delta
	}
}

// fold is the verdict of the counted Tasks alone.
func (s stateCounts) fold() verdictFold {
	return verdictFold{
		running: s.running > 0, failed: s.failed > 0, blocked: s.blocked > 0,
		cancelled: s.cancelled > 0, unresolved: s.unresolved > 0,
	}
}

// seenEntry is one Task's first-paint stamp.
type seenEntry struct {
	at time.Time
	t  *taskState
}

// seenHeap is the first-paint stamps of a set of Tasks, earliest on top.
// A Task that left the set leaves its entry behind; earliest discards
// those when they surface.
type seenHeap []seenEntry

func (h seenHeap) Len() int           { return len(h) }
func (h seenHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h seenHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *seenHeap) Push(x any)        { *h = append(*h, x.(seenEntry)) }
func (h *seenHeap) Pop() any {
	old := *h
	e := old[len(old)-1]
	*h = old[:len(old)-1]
	return e
}

// earliest is the earliest stamp of a Task still in the set, per member,
// or the zero time.
func (h *seenHeap) earliest(member func(*taskState) bool) time.Time {
	for h.Len() > 0 {
		top := (*h)[0]
		if member(top.t) && top.t.filing.seen.Equal(top.at) {
			return top.at
		}
		heap.Pop(h)
	}
	return time.Time{}
}
