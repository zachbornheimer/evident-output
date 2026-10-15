package project

import (
	"maps"
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Tally is the counted partition of a Group's Excluded item Tasks: how
// many, split by Exclude reason in first-seen order, with the item names
// under each reason. It is the one owner of that count, so the headline and
// its reason parts always sum.
//
// Building one is a single O(items) pass with no sorting, so a renderer may
// rebuild it every frame.
type Tally struct {
	reasons []ReasonTally
	index   map[string]int
	total   int
}

// ReasonTally is one Reason's share of a Tally, with its item names in
// declaration order. Facts is aligned with Names: the Facts of each item
// Task (nil for an item with none).
type ReasonTally struct {
	Reason string
	Names  []string
	Facts  [][]record.Fact
}

// HasFacts reports whether any item under this Reason carries Facts.
func (r ReasonTally) HasFacts() bool {
	for _, f := range r.Facts {
		if len(f) > 0 {
			return true
		}
	}
	return false
}

// AddItem counts t, an Excluded item of a Group (IsExcludedItem), under its
// Exclude reason, carrying t's Facts.
func (t *Tally) AddItem(item *record.TaskSnapshot) {
	reason, _ := ExcludeReason(item)
	t.add(reason, item.Name, item.Facts)
}

// add counts name under reason, recording facts as its item's Facts.
func (t *Tally) add(reason, name string, facts []record.Fact) {
	if t.index == nil {
		t.index = make(map[string]int)
	}
	i, seen := t.index[reason]
	if !seen {
		i = len(t.reasons)
		t.index[reason] = i
		t.reasons = append(t.reasons, ReasonTally{Reason: reason})
	}
	r := &t.reasons[i]
	r.Names = append(r.Names, name)
	if len(facts) > 0 || r.Facts != nil {
		for len(r.Facts) < len(r.Names)-1 {
			r.Facts = append(r.Facts, nil)
		}
		r.Facts = append(r.Facts, facts)
	}
	t.total++
}

// Total is how many items the Tally counted.
func (t Tally) Total() int { return t.total }

// Empty reports whether the Tally counted nothing.
func (t Tally) Empty() bool { return t.total == 0 }

// Reasons is the partition, in the order each Reason was first recorded.
func (t Tally) Reasons() []ReasonTally { return t.reasons }

// Clone is a Tally the caller can hold after the original is mutated.
// Slice and map fields are copied, so a live index can keep appending
// while a frame reads this copy.
func (t Tally) Clone() Tally {
	out := Tally{total: t.total}
	if n := len(t.reasons); n > 0 {
		out.reasons = make([]ReasonTally, n)
		for i, r := range t.reasons {
			out.reasons[i] = ReasonTally{
				Reason: r.Reason,
				Names:  slices.Clone(r.Names),
			}
			if len(r.Facts) > 0 {
				facts := make([][]record.Fact, len(r.Facts))
				for j, f := range r.Facts {
					facts[j] = record.CloneFacts(f)
				}
				out.reasons[i].Facts = facts
			}
		}
	}
	if len(t.index) > 0 {
		out.index = maps.Clone(t.index)
	}
	return out
}
