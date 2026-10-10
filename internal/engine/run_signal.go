package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (wired to SIGINT/SIGTERM by those
// entrypoints) in place of the pre-v0.6 no-context func() error form.
type RunFunc func(context.Context) error

// signalNotifier and signalStopper abstract signal delivery (facade rule) so
// SIGINT/SIGTERM handling in Main is exercised in tests without sending
// real process signals.
type (
	signalNotifier func(c chan<- process.Signal, sig ...process.Signal)
	signalStopper  func(c chan<- process.Signal)
)

var (
	notifySignals signalNotifier = process.Notify
	stopSignals   signalStopper  = process.StopNotify
)

// signalChannelCapacity holds one signal while cancelActive is in flight plus
// one more so a second Ctrl-C arriving during cleanup is never dropped.
const signalChannelCapacity = 2
