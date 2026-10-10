package engine

import (
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// scheduler is the run's execution state: the callbacks in flight, the
// goroutines parked in Wait, and the flags that decide whether anything new
// may start. Guarded by Output.mu. Which Tasks are eligible, and where each
// stands, is the graph's (see graph.Graph); the methods that drive this live
// on Output (execution.go, wait.go, stall.go), because every decision also
// reads Task state.
type scheduler struct {
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
	// callbackGoroutines counts, per goroutine, the task callbacks it is
	// running right now. A parked goroutine that one of them started may be
	// what that callback is blocked on (see heldCallbacksLocked).
	callbackGoroutines map[graph.GoroutineID]int
	// consumers is, per goroutine, the stack of Tasks and builder gates it
	// is running (see enterConsumer).
	consumers map[graph.GoroutineID][]*taskState
	// cancelled stops dispatching anything new: after an interrupt the
	// queue is abandoned, not drained.
	cancelled bool
}
