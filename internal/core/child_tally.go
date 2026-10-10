package core

import (
	"time"
	"unicode/utf8"
)

// ChildTally is what a live projection of a collection knows about every
// child Task it was built from, including the ones it left out of Tasks.
// A live frame shows at most a screen of rows, so a collection of 16000
// Tasks is projected as the few children the frame could select plus this
// tally, and frame work stays bound by the screen rather than the run.
type ChildTally struct {
	// All counts every child Task.
	All ChildCounts
	// Folded reports whether the collection's disposition items fold into
	// Items (the renderer's rule, decided over every child).
	Folded bool
	// Items sums the folded items' Skipped and Kept records.
	Items Dispositions
	// Rest counts the children that remain once items fold: All when
	// they do not.
	Rest ChildCounts
}

// ChildCounts summarizes a list of child Tasks.
type ChildCounts struct {
	// Total is every child; Done is those that completed (Done or Skipped).
	Total, Done int
	// Running, Pending and Unfinished report whether any child is Running,
	// Pending, or not yet terminal.
	Running, Pending, Unfinished bool
	// EarliestSeen is the earliest non-zero LiveFirstSeenAt among them.
	EarliestSeen time.Time
	// NameWidth is the widest Name among them, in runes.
	NameWidth int
}

// Add counts t.
func (c *ChildCounts) Add(t *TaskSnapshot) {
	c.Total++
	switch t.State {
	case Done, Skipped:
		c.Done++
	case Running:
		c.Running = true
	case Pending:
		c.Pending = true
	}
	if !IsTerminalTask(t.State) {
		c.Unfinished = true
	}
	c.NameWidth = max(c.NameWidth, utf8.RuneCountInString(t.Name))
	if seen := t.LiveFirstSeenAt(); !seen.IsZero() && (c.EarliestSeen.IsZero() || seen.Before(c.EarliestSeen)) {
		c.EarliestSeen = seen
	}
}

// Merge adds everything o counted.
func (c *ChildCounts) Merge(o ChildCounts) {
	c.Total += o.Total
	c.Done += o.Done
	c.Running = c.Running || o.Running
	c.Pending = c.Pending || o.Pending
	c.Unfinished = c.Unfinished || o.Unfinished
	c.NameWidth = max(c.NameWidth, o.NameWidth)
	if !o.EarliestSeen.IsZero() && (c.EarliestSeen.IsZero() || o.EarliestSeen.Before(c.EarliestSeen)) {
		c.EarliestSeen = o.EarliestSeen
	}
}

// CountTasks summarizes tasks.
func CountTasks(tasks []TaskSnapshot) ChildCounts {
	var c ChildCounts
	for i := range tasks {
		c.Add(&tasks[i])
	}
	return c
}

// WithChildTally is col whose Tasks are a partial list tallied by t.
func WithChildTally(col TasksSnapshot, t ChildTally) TasksSnapshot {
	tallies := col.Tallies()
	tallies.Children = &t
	return col.WithTallies(tallies)
}

// WithoutChildTally is col whose Tasks are its complete child list.
func WithoutChildTally(col TasksSnapshot) TasksSnapshot {
	tallies := col.Tallies()
	tallies.Children = nil
	return col.WithTallies(tallies)
}

// ChildTallyOf is the tally of col's partial Tasks list, or false when
// Tasks is complete.
func ChildTallyOf(col TasksSnapshot) (ChildTally, bool) {
	tally, ok := col.Tallies().Children.(*ChildTally)
	if !ok || tally == nil {
		return ChildTally{}, false
	}
	return *tally, true
}

// WithRootTally is s whose standalone root Tasks are a partial list
// tallied by t.
func WithRootTally(s Snapshot, t ChildTally) Snapshot {
	tallies := s.RootTallies()
	tallies.Children = &t
	return s.WithRootTallies(tallies)
}

// RootTallyOf is the tally of s's partial root Tasks list, or false when
// Tasks is complete.
func RootTallyOf(s Snapshot) (ChildTally, bool) {
	tally, ok := s.RootTallies().Children.(*ChildTally)
	if !ok || tally == nil {
		return ChildTally{}, false
	}
	return *tally, true
}

// CollectionTally is what a live projection of a collection knows about
// the child collections it left out of Collections: a frame paints nested
// collections in declaration order and reaches at most one more of them
// than it has rows, so the rest need only be counted.
type CollectionTally struct {
	// Count is how many child collections were left out.
	Count int
	// Tasks counts every Task at or below them.
	Tasks ChildCounts
	// Settled is how many of them hold no unfinished Task.
	Settled int
	// OwnRows is how many of them render as one own-Task row, and
	// OwnRowNameWidth the widest such row's name, in runes: they still
	// set a header-less parent's name column.
	OwnRows, OwnRowNameWidth int
}

// Empty reports whether the tally counts nothing.
func (t CollectionTally) Empty() bool { return t.Count == 0 }

// WithCollectionTally is col whose Collections are a partial list tallied
// by t.
func WithCollectionTally(col TasksSnapshot, t CollectionTally) TasksSnapshot {
	tallies := col.Tallies()
	tallies.Collections = collectionTallySlot(t)
	return col.WithTallies(tallies)
}

// CollectionTallyOf is the tally of the child collections col's
// projection left out; the zero tally when Collections is complete.
func CollectionTallyOf(col TasksSnapshot) CollectionTally {
	return collectionTallyIn(col.Tallies())
}

// WithRootCollectionTally is s whose root collections are a partial list
// tallied by t.
func WithRootCollectionTally(s Snapshot, t CollectionTally) Snapshot {
	tallies := s.RootTallies()
	tallies.Collections = collectionTallySlot(t)
	return s.WithRootTallies(tallies)
}

// RootCollectionTallyOf is the tally of the root collections s's
// projection left out; the zero tally when Collections is complete.
func RootCollectionTallyOf(s Snapshot) CollectionTally {
	return collectionTallyIn(s.RootTallies())
}

// collectionTallySlot is what a snapshot stores for t: nothing when t counts
// nothing, so a complete Collections list carries no tally.
func collectionTallySlot(t CollectionTally) any {
	if t.Empty() {
		return nil
	}
	return &t
}

// collectionTallyIn is the collection tally stored in tallies, zero when none.
func collectionTallyIn(tallies Tallies) CollectionTally {
	tally, ok := tallies.Collections.(*CollectionTally)
	if !ok || tally == nil {
		return CollectionTally{}
	}
	return *tally
}
