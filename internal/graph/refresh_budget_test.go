package graph

import "testing"

// spend takes dumps until the budget refuses, and counts them.
func spend(b *refreshBudget) int {
	n := 0
	for b.take() {
		n++
	}
	return n
}

// Each callback that begins earns the run its allowance, so a second callback
// begun after the first spent some does not leave the first with a share that
// the second's own lookups cannot cover.
func TestRefreshBudgetEarnsOneAllowanceForEachRunningCallback(t *testing.T) {
	var b refreshBudget
	b.settle(1, refreshesPerCallback)
	b.take()
	b.take()
	b.settle(2, refreshesPerCallback)
	if got, want := spend(&b), 2*refreshesPerCallback-2; got != want {
		t.Fatalf("two running callbacks, two dumps spent: %d dumps left, want %d", got, want)
	}
}

// A callback that finished leaves nothing behind: the work still running holds
// only its own share, however many callbacks ran before it.
func TestRefreshBudgetKeepsNothingForCallbacksThatFinished(t *testing.T) {
	var b refreshBudget
	for range 100 {
		b.settle(1, refreshesPerCallback)
		b.settle(0, 0)
	}
	b.settle(1, refreshesPerCallback)
	if got := spend(&b); got != refreshesPerCallback {
		t.Fatalf("one running callback after 100 finished: %d dumps, want %d", got, refreshesPerCallback)
	}
}

// No callback running holds no dump, and a nil budget has no limit.
func TestRefreshBudgetOfNoRunningCallbackIsEmpty(t *testing.T) {
	var b refreshBudget
	b.settle(0, refreshesPerCallback)
	if b.take() {
		t.Fatal("a budget with no running callback allowed a dump")
	}
	var unlimited *refreshBudget
	if !unlimited.take() {
		t.Fatal("a nil budget refused a dump")
	}
}
