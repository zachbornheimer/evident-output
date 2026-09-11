package evo

import (
	"iter"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func eachChildren(out *Output, groupID string, items []string) iter.Seq2[string, *TaskHandle] {
	return func(yield func(string, *TaskHandle) bool) {
		if out == nil {
			return
		}
		if !out.sealEachTotal(groupID) {
			return
		}
		tasks := out.addEachChildren(groupID, items)
		var wait []*TaskHandle
		for i, item := range items {
			if i >= len(tasks) {
				break
			}
			task := tasks[i]
			cont := yield(item, task)
			if task.wasSubmitted() {
				wait = append(wait, task)
			}
			if !cont {
				break
			}
		}
		for _, task := range wait {
			task.waitSubmitted()
		}
	}
}

// sealEachTotal claims a collection's one Each denominator and reports
// whether this call may declare children. A collection derives its
// completed/total from its Each children, so a second Each changes a total
// the reader has already been shown: one subject declared its delete Each,
// rendered `✓ branches  25/25`, then declared its kept Each and rendered
// `✓ branches  145/145` a second later. The dialect forbids exactly that
// ("a sealed total never changes ... Never 14/40 -> 14/53"), so the second
// call is recorded misuse and yields nothing rather than re-opening a
// number the reader already trusted.
//
// Only Each seals. Declaring explicitly named children one at a time
// (group.Task("lint"), group.Task("test")) is the ordinary Group shape and
// is untouched — those children are semantically named work, not items of a
// counted collection, and never enter the denominator.
//
// The caller's correct spelling for a partitioned collection is one Each
// over every item, resolving each child as deleted or kept, which is also
// what makes the taxonomy partition sum.
func (o *Output) sealEachTotal(groupID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.tasksByRef[groupID]
	if col == nil {
		return false
	}
	if col.eachSealed {
		o.recordMisuseFor(col.name, ErrAlreadyResolved)
		return false
	}
	col.eachSealed = true
	return true
}

// addEachChildren declares every item as a child under one lock, then
// paints once, so the first live frame already shows 0/N. Yielding one
// child at a time used to grow the denominator as the loop ran
// (`141/144` becoming `145/145`).
func (o *Output) addEachChildren(groupID string, items []string) []*TaskHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.tasksByRef[groupID]
	if col == nil {
		handles := make([]*TaskHandle, len(items))
		for i := range items {
			handles[i] = &TaskHandle{out: o, id: o.nextID("task")}
		}
		return handles
	}
	handles := make([]*TaskHandle, 0, len(items))
	for _, name := range items {
		handles = append(handles, o.declareTaskLocked(txt.Text(name), col, "", true))
	}
	o.signalLiveLocked(true)
	return handles
}
