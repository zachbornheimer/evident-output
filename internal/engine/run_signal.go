package engine

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (SIGINT/SIGTERM, or an embedded
// Output's caller ctx ending; see Output.Run) in place of the pre-v0.6
// no-context func() error form.
type RunFunc func(context.Context) error

// signalNotifier and signalStopper abstract os/signal (facade rule) so
// SIGINT/SIGTERM handling in Main is exercised in tests without sending
// real process signals.
type signalNotifier func(c chan<- os.Signal, sig ...os.Signal)
type signalStopper func(c chan<- os.Signal)

var (
	notifySignals signalNotifier = signal.Notify
	stopSignals   signalStopper  = signal.Stop
)

// signalChannelCapacity holds one signal while cancelActive is in flight plus
// one more so a second Ctrl-C arriving during cleanup is never dropped.
const signalChannelCapacity = 2

// signalWindow is how long a run acts on SIGINT/SIGTERM.
type signalWindow uint8

const (
	// signalsHostOwned: an Embedded run registers no handler. Its host owns
	// process signals; an HTTP server's SIGTERM means graceful shutdown,
	// which must let in-flight requests finish (spec §53).
	signalsHostOwned signalWindow = iota
	// signalsUntilCallbackReturns: a FormatExternal run that did not opt
	// into Embedded keeps the 1.1 window (DEC-CANCEL-005 owner rule). The
	// handler stays registered through Finish, but a signal that arrives
	// after the run callback returned is caught and ignored, so the Define
	// work Finish waits on completes.
	signalsUntilCallbackReturns
	// signalsUntilConcluded: every CLI format. A ^C stops the run at any
	// point before it concludes, including Define work running in Finish.
	signalsUntilConcluded
)

// signalWindow resolves this Output's window from its lifecycle owner and
// its format.
func (o *Output) signalWindow() signalWindow {
	switch {
	case o.cfg.embedded:
		return signalsHostOwned
	case o.cfg.external:
		return signalsUntilCallbackReturns
	default:
		return signalsUntilConcluded
	}
}

// processSignals is one run's SIGINT/SIGTERM subscription. received is nil
// when the embedding host owns process signals; a nil channel never fires
// in a select, so the run then stops only through its caller's context.
type processSignals struct {
	received chan os.Signal
	window   signalWindow
}

// subscribeProcessSignals registers this run for SIGINT/SIGTERM unless the
// Output is Embedded (Config.Embedded).
func (o *Output) subscribeProcessSignals() processSignals {
	window := o.signalWindow()
	if window == signalsHostOwned {
		return processSignals{window: window}
	}
	received := make(chan os.Signal, signalChannelCapacity)
	notifySignals(received, syscall.SIGINT, syscall.SIGTERM)
	return processSignals{received: received, window: window}
}

// stop releases the subscription; a host-owned one holds nothing.
func (s processSignals) stop() {
	if s.received != nil {
		stopSignals(s.received)
	}
}

// awaitInterrupt waits for the run to conclude or for the first signal this
// run's window acts on. concluded reports which one happened.
func (s processSignals) awaitInterrupt(results <-chan Result, phase *runPhase) (result Result, concluded bool) {
	received := s.received
	for {
		select {
		case result := <-results:
			return result, true
		case <-received:
			if phase.signal() || s.window == signalsUntilConcluded {
				return Result{}, false
			}
			// The callback already returned: the 1.1 window is closed, and
			// every later signal is caught and ignored.
			received = nil
		}
	}
}

// runPhase orders the run callback returning against the first signal, so
// exactly one of them decides whether the callback was interrupted.
type runPhase struct{ state atomic.Int32 }

const (
	phaseCallbackRunning int32 = iota
	phaseCallbackReturned
	phaseSignalled
)

// callbackReturned records that the run callback returned and reports
// whether a signal interrupted it first.
func (p *runPhase) callbackReturned() (signalled bool) {
	return !p.state.CompareAndSwap(phaseCallbackRunning, phaseCallbackReturned)
}

// signal records a signal and reports whether it arrived while the run
// callback was still running.
func (p *runPhase) signal() (duringCallback bool) {
	return p.state.CompareAndSwap(phaseCallbackRunning, phaseSignalled)
}
