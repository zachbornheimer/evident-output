package engine

// Wait blocks until the task is terminal and returns the error its callback
// returned (nil on Done, Skipped, or a dry run that never invoked it).
//
// A task that was already resolved before it was defined never runs its
// callback (the misuse is recorded at the Define call), and Wait returns nil
// immediately rather than blocking on work that will never happen (P15).
//
// A task that never ran — an abandoned queue, a predecessor that failed —
// returns ErrNotStarted rather than nil, and a cancelled one returns its
// cancellation: Wait never reports success for work that did not happen.
//
// Wait called while the calling goroutine holds a resource claim (inside an
// Effect, File, or Basis hold) returns ErrNestedResourceAcquisition without
// waiting: the awaited work could need that claim, and neither could move.
//
// A waiter has stopped doing work, so the concurrency ceiling must not be
// the reason the task it waits on cannot start (P16). Wait therefore runs
// that task, and whatever is holding it back, on its own goroutine when the
// scheduler has not picked them up (see graph.Graph.RunWaited): nested
// Define+Wait completes even at MaxConcurrency 1, because the waiting
// callback's slot carries the work it is waiting for instead of idling.
//
// MaxConcurrency bounds every executing callback. A plain caller (one
// outside any callback) holds no slot, so it runs work only in a free slot
// and otherwise waits for the pool. A goroutine a callback started holds
// none either: if that callback blocks on it while every slot is held,
// neither can move, and Wait returns ErrWaitDeadlock naming the shape
// instead of hanging. Declare that work as Tasks under a Group, or Wait
// inside the callback itself (API-041).
func (t *TaskHandle) Wait() error {
	if t == nil || t.out == nil {
		return nil
	}
	if t.rejected != nil {
		return rejectedWaitOutcome(t.rejected)
	}
	node := t.out.graph.Task(t.id)
	if node == nil {
		return nil
	}
	outcome := t.out.graph.Wait(node)
	t.out.followRecordAfterWait()
	return outcome
}

// followRecordAfterWait is what a Wait owes its caller once the graph let it
// go: the render state has answered every settle the Wait saw. A Task's done
// channel closes inside the graph's critical section, before the resolution
// that settled it has painted and filed the row. Taking o.mu here waits out
// that resolution (it holds o.mu from settle to commit) and follows the
// record for a settle made outside one, so a caller that reads the frame
// right after Wait sees the settled row.
func (o *Output) followRecordAfterWait() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
}
