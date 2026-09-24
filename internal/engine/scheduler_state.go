package engine

import "sync"

// scheduler is the run's scheduling state: the Tasks ready to start, the
// callbacks in flight, the goroutines parked in Wait, and the flags that
// decide what may start next. Guarded by Output.mu. The methods that drive
// it live on Output (eligibility.go, execution.go, wait.go, stall.go),
// because every decision also reads Task state.
type scheduler struct {
	// queue holds the Tasks ready to start (see schedQueue).
	queue schedQueue
	// parked counts Tasks waiting off the queue on a predecessor.
	parked int
	// woken is the worklist wakeLocked drains; waking marks it in use.
	woken  []*taskState
	waking bool
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
	// waits holds one ticket per goroutine parked in TaskHandle.Wait.
	// Together with executing it decides whether the run can still
	// progress, and it is how a wait that never can be satisfied is
	// released instead of hanging Finish (see resolveStall).
	waits map[*waitTicket]struct{}
	// draining is set once Finish starts running the queue to empty.
	draining bool
	// cancelled stops dispatching anything new: after an interrupt the
	// queue is abandoned, not drained.
	cancelled bool
	// predChecks counts predecessor outcomes read, so a test can prove
	// fan-in scheduling stays linear.
	predChecks int
	// followerChecks counts Sequence followers failSequenceFollowers
	// examined, so a test can prove repeated failures stay linear.
	followerChecks int
}
