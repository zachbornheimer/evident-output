package engine

import (
	"context"
	"os"
	"os/signal"
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

// processSignals is one run's SIGINT/SIGTERM subscription. received is nil
// when the embedding host owns process signals; a nil channel never fires
// in a select, so the run then stops only through its caller's context.
type processSignals struct {
	received chan os.Signal
}

// subscribeProcessSignals registers this run for SIGINT/SIGTERM unless the
// Output is embedded (FormatExternal): an HTTP server's SIGTERM means
// graceful shutdown, which must let in-flight requests finish (spec §53).
func (o *Output) subscribeProcessSignals() processSignals {
	if o.cfg.embedded {
		return processSignals{}
	}
	received := make(chan os.Signal, signalChannelCapacity)
	notifySignals(received, syscall.SIGINT, syscall.SIGTERM)
	return processSignals{received: received}
}

// stop releases the subscription; a host-owned one holds nothing.
func (s processSignals) stop() {
	if s.received != nil {
		stopSignals(s.received)
	}
}
