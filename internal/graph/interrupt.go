package graph

import "context"

// cancellationScope is the run's own cancellation signal, the thing a
// callback doing I/O selects on. It lasts one Run: BeginRunContext replaces
// it, cancelling the one before. Guarded by the Graph's mutex.
type cancellationScope struct {
	cancel context.CancelFunc
	// signalOf yields the scope's context. The scope hands out the context it
	// owns instead of storing it as a field, so nothing outside it can keep
	// a context that outlives the scope.
	signalOf func() context.Context
}

// runStop is how the run was stopped: who did it, and the abort channel of
// each pending Confirm gate. Guarded by the Graph's mutex.
type runStop struct {
	// cause names who stopped the run ("by user" for a signal). It becomes
	// the cancelled Conclusion's Explanation, so the band and the JSON
	// document state the same cause.
	cause string
	// aborts holds one abort channel per pending Confirm gate, keyed by the
	// gate Task's id, so an interrupt can unblock the stdin read and let the
	// gate settle Cancelled (not Blocked "declined").
	aborts map[string]chan struct{}
}

func newCancellationScope(parent context.Context) cancellationScope {
	signal, cancel := context.WithCancel(parent)
	return cancellationScope{cancel: cancel, signalOf: func() context.Context { return signal }}
}

// processRoot is the parent of a run's context before any caller supplies
// one: nothing cancels it, so the run is cancelled only by Interrupt, Cancel
// or a later BeginRunContext.
func processRoot() context.Context { return context.Background() }

// Context is the run's cancellation signal. It is cancelled when the run is
// interrupted and when the graph is cancelled, so work that does I/O can
// select on it and stop instead of running on past the ^C that was supposed
// to end it.
func (g *Graph) Context() context.Context {
	g.lock()
	defer g.unlock()
	return g.scope.signalOf()
}

// BeginRunContext makes parent (the root when nil) the parent of the run's
// context from now on, replacing the placeholder the graph started with, so
// a caller's own cancellation or deadline reaches every task scope started
// afterwards. The previous context is cancelled, not left to leak: nothing
// after this point should still be watching it. It returns the new context.
func (g *Graph) BeginRunContext(parent context.Context) context.Context {
	if parent == nil {
		parent = processRoot()
	}
	next := newCancellationScope(parent)
	g.lock()
	previous := g.scope
	g.scope = next
	g.unlock()
	previous.cancel()
	return next.signalOf()
}

// Cancel cancels the run's context.
func (g *Graph) Cancel() {
	g.lock()
	cancel := g.scope.cancel
	g.unlock()
	cancel()
}

// CancelCause is who stopped the run, empty when nobody did.
func (g *Graph) CancelCause() string {
	g.lock()
	defer g.unlock()
	return g.stop.cause
}

// Interrupt stops the run in the one order that leaves the ledger honest: the
// scheduler is closed to new work, every row is put into the state the reader
// must see (cancelRows, the caller's step), and only then is the run's
// context cancelled to release the callbacks still in flight. Cancelling
// first would race a finishing callback into a success row after the ^C.
// cause is who stopped the run. The caller holds no lock cancelRows takes.
func (g *Graph) Interrupt(cause string, cancelRows func()) {
	g.lock()
	g.exec.cancelled = true
	g.stop.cause = cause
	g.unlock()

	cancelRows()
	g.abandonQueued()
	g.Cancel()
}

// abandonQueued settles every Task that had not begun NotStarted: the
// interrupt's answer to "and what about the rest?", which the reader would
// otherwise never get.
//
// Submitted or not: a Task the caller declared and never Defined is work the
// interrupt took away just as surely as one sitting in the scheduler's queue.
// Sweeping only the submitted ones left a declared row Pending, and Finish
// then charged the caller with an unresolved Task about a run the user had
// just cancelled.
func (g *Graph) abandonQueued() {
	g.lock()
	defer g.unlock()
	for _, t := range g.taskList {
		if t.sched.phase != PhaseRunning && !terminal(t) {
			g.markNotStartedLocked(t)
		}
	}
	for _, gate := range g.gates {
		if gate.sched.phase != PhaseRunning && !terminal(gate) {
			g.markNotStartedLocked(gate)
		}
	}
}

// RegisterAbort registers an abort channel for the Confirm gate id, before
// its prompt is written, and returns it with the func that ends the
// registration. Registering late would leave a window where an interrupt had
// no channel to close and the stdin read could never be unblocked.
func (g *Graph) RegisterAbort(id string) (abort <-chan struct{}, release func()) {
	ch := make(chan struct{})
	g.lock()
	if g.stop.aborts == nil {
		g.stop.aborts = make(map[string]chan struct{})
	}
	g.stop.aborts[id] = ch
	g.unlock()
	return ch, func() {
		g.lock()
		defer g.unlock()
		delete(g.stop.aborts, id)
	}
}

// AbortPending closes the abort channel of one pending Confirm gate and
// forgets it, so an interrupt unblocks the gate's stdin read. It returns the
// gate's id, or false when no gate is pending.
func (g *Graph) AbortPending() (id string, ok bool) {
	g.lock()
	defer g.unlock()
	for id, abort := range g.stop.aborts {
		close(abort)
		delete(g.stop.aborts, id)
		return id, true
	}
	return "", false
}

// PendingAborts is how many Confirm gates are waiting on an answer.
func (g *Graph) PendingAborts() int {
	g.lock()
	defer g.unlock()
	return len(g.stop.aborts)
}
