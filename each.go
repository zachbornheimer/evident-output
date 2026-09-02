package evo

import "iter"

// Each iterates items, driving absolute Progress(i+1, len(items)) and
// Phase(item) before each item is yielded. Evo owns the counter: because
// progress is set from the loop index rather than a hand-maintained counter,
// re-running work for an item inside the loop body cannot double-count or
// move the bar backward — only advancing to the next item does.
//
// Breaking out of the loop early leaves progress at the count already
// reached; the task is not auto-resolved (call Done/Fail/etc. explicitly).
func (t *TaskHandle) Each(items []string) iter.Seq[string] {
	total := len(items)
	return func(yield func(string) bool) {
		for i, item := range items {
			t.Progress(i+1, total)
			t.Phase(item)
			if !yield(item) {
				return
			}
		}
	}
}

// EachN iterates a count-only loop with no item names, driving absolute
// Progress(i+1, n) before each index is yielded. Use Each when items have
// names worth showing as the phase.
func (t *TaskHandle) EachN(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			t.Progress(i+1, n)
			if !yield(i) {
				return
			}
		}
	}
}
