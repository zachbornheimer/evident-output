package engine

import (
	"context"
	"os"
	"os/signal"
)

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (wired to SIGINT/SIGTERM by those
// entrypoints) in place of the pre-v0.6 no-context func() error form.
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
