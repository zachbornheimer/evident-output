package evo

import "iter"

// Each iterates items, driving absolute Progress(i, len(items)) and a phase
// default of item before each item is yielded — the bar reads "items
// completed so far", not "items completed including the one still
// running", so it never shows full while the last item is in flight. Evo
// owns the counter: because progress is set from the loop index rather
// than a hand-maintained counter, re-running work for an item inside the
// loop body cannot double-count or move the bar backward — only advancing
// to the next item does. Normal completion of the loop seals progress at
// total/total.
//
// The item-name phase is a courtesy default, not a declared phase: if the
// loop body calls Phase itself before the next paint, that call's own text
// is what streams (in plain mode) — the bare item name never forces its
// own redundant durable line first (beginner-10).
//
// Breaking out of the loop early leaves progress at the count already
// reached; the task is not auto-resolved (call Done/Fail/etc. explicitly).
func (t *TaskHandle) Each(items []string) iter.Seq[string] {
	total := len(items)
	return func(yield func(string) bool) {
		for i, item := range items {
			t.Progress(i, total)
			t.autoPhase(item)
			if !yield(item) {
				return
			}
		}
		t.Progress(total, total)
	}
}

// EachN iterates a count-only loop with no item names, driving absolute
// Progress(i, n) before each index is yielded — see Each for why the bar
// reads the count completed so far rather than including the in-flight
// item. Use Each when items have names worth showing as the phase. Normal
// completion of the loop seals progress at n/n.
func (t *TaskHandle) EachN(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			t.Progress(i, n)
			if !yield(i) {
				return
			}
		}
		t.Progress(n, n)
	}
}
