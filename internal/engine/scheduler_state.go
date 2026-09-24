package engine

import "sync"

// scheduler is the run's scheduling state: the Tasks submitted but not
// started, the callbacks in flight, the goroutines parked in Wait, and the
// flags that decide what may start next. Guarded by Output.mu. The methods
// that drive it live on Output (scheduler.go), because every decision also
// reads Task state.
type scheduler struct {
	// queue holds submitted Tasks nobody has started (see schedQueue).
	queue schedQueue
	// wg counts submitted work that has not settled; the drain waits on it.
	wg sync.WaitGroup
	// inflight counts pooled worker slots in use, bounded by the
	// concurrency ceiling; maxObserved is its high-water mark.
	inflight    int
	maxObserved int
	// executing counts task callbacks currently running, pooled and donated
	// alike — inflight counts only the pooled slots, so it cannot answer
	// "is any callback still moving?".
	executing int
	// startOrder is every started Task's name, in start order.
	startOrder []string
	// waits holds one ticket per goroutine parked in TaskHandle.Wait.
	// Together with executing it decides whether the run can still
	// progress, and it is how a wait that never can be satisfied is
	// released instead of hanging Finish (see releaseUnsatisfiableWaits).
	waits map[*waitTicket]struct{}
	// draining is set once Finish starts running the queue to empty.
	draining bool
	// cancelled stops dispatching anything new: after an interrupt the
	// queue is abandoned, not drained.
	cancelled bool
	// cascadeDue records that queued dependents may have become unreachable
	// since the last NotStarted cascade (see cascadeIneligibleLocked).
	cascadeDue bool
}
