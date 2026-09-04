package evo

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// signalNotifier and signalStopper abstract os/signal (facade rule) so
// SIGINT/SIGTERM handling in Main/MainWith is exercised in tests without
// sending real process signals.
type signalNotifier func(c chan<- os.Signal, sig ...os.Signal)
type signalStopper func(c chan<- os.Signal)

var (
	notifySignals signalNotifier = signal.Notify
	stopSignals   signalStopper  = signal.Stop
)

// signalChannelCapacity holds one signal while cancelActive is in flight plus
// one more so a second Ctrl-C arriving during cleanup is never dropped.
const signalChannelCapacity = 2

// Run executes a CLI presentation lifecycle against this Output and returns
// the process exit code — the Isolated-instance counterpart of Main, for a
// caller holding its own *Output (evo.Init(evo.Config{Isolated: true})).
//
// Typical entrypoint:
//
//	func main() {
//	    out := evo.Init(evo.Config{Title: "tool", Isolated: true})
//	    os.Exit(out.Run(run))
//	}
//
// Lifecycle: arm first paint → run → (reconcile run error into model) →
// Finish → Close.
//
// Exit codes:
//   - nil Output → ExitFailed (2)
//   - SIGINT/SIGTERM → Cancel on the active task (or the output) → ExitCancelled (130)
//   - a second SIGINT/SIGTERM → ExitCancelled (130) returned immediately, without
//     waiting for run to unwind, so the caller's os.Exit(out.Run(...)) exits now
//   - Finish/Close bookkeeping misuse (a leftover unresolved task, a
//     double-resolve, ...) is folded into the Conclusion before it renders,
//     so the printed band and Conclusion.ExitCode already agree; a Blocked
//     conclusion keeps ExitBlocked (1) regardless — the documented
//     "Block → exit 1" contract wins over a leftover bookkeeping misuse
//   - a genuine renderer/write failure (surfacing only after the band is
//     already flushed) still escalates an otherwise-OK exit code to
//     ExitFailed (2)
//   - otherwise Conclusion.ExitCode after reconciling run errors into Fail
//
// Config.FailedExitCode (when non-zero) overrides ExitFailed for a failed
// conclusion so CLIs that contract on exit 1 can set FailedExitCode: 1.
//
// A non-nil application error is recorded as an output-level Fail before Finish
// so the human conclusion cannot show [ready] while the process fails.
func (o *Output) Run(run func(*Output) error) int {
	if o == nil {
		return ExitFailed
	}
	o.arm()
	return runInterruptible(o, run)
}

// Main runs a CLI presentation lifecycle against the package-level default
// instance (see Init) and returns the process exit code.
//
//	func main() {
//	    evo.Init(evo.Config{Title: "tool"})
//	    os.Exit(evo.Main(run))
//	}
//
// run reports only an error; the Conclusion (0/1/2/130) is the sole source of
// the exit code — see Output.Run for the full lifecycle and signal contract.
func Main(run func() error) int {
	out := Default()
	out.arm()
	return runInterruptible(out, func(o *Output) error {
		if run == nil {
			return nil
		}
		return run()
	})
}

// runInterruptible executes run to completion, wiring SIGINT/SIGTERM into
// cancellation of the active task (or the output itself) so the ledger and
// exit code always agree. A second signal returns ExitCancelled immediately
// instead of waiting for run to unwind — the process-level os.Exit that wraps
// Main/MainWith is what actually terminates.
func runInterruptible(out *Output, run func(*Output) error) int {
	sigCh := make(chan os.Signal, signalChannelCapacity)
	notifySignals(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals(sigCh)

	done := make(chan error, 1)
	go func() {
		var err error
		if run != nil {
			err = run(out)
		}
		done <- err
	}()

	select {
	case runErr := <-done:
		return concludeRun(out, runErr)
	case <-sigCh:
		out.cancelActive("interrupted")
		select {
		case <-done:
			return concludeCancelled(out)
		case <-sigCh:
			return ExitCancelled
		}
	}
}

// concludeRun reconciles an ordinary (non-interrupted) run outcome into the
// process exit code.
func concludeRun(out *Output, runErr error) int {
	if runErr != nil && !out.AnyFailed() {
		// Synchronize the presentation model with the application error only when
		// no entity already recorded Failed — avoids a duplicate synthetic Fail row
		// on top of an existing task/item Fail. Exit code still comes from conclusion.
		out.Failf("command failed: %w", runErr)
	}
	finishErr := out.Finish()
	closeErr := out.Close()
	code := out.Conclusion().ExitCode
	// Bookkeeping misuse (a leftover unresolved task, a duplicate key, ...)
	// is already folded into the Conclusion itself before the band renders
	// (release-gate finding 2) — ExitCode already agrees with what printed.
	// A genuine renderer/write failure is different: it surfaces only after
	// the band is already flushed, so the band never had a chance to
	// reflect it — that still escalates an otherwise-OK exit code here.
	if code == ExitOK && (errors.Is(finishErr, ErrRenderer) || errors.Is(closeErr, ErrRenderer)) {
		return ExitFailed
	}
	return code
}

// concludeCancelled finalizes an interrupted run; the Conclusion computed
// from the Cancel already recorded is the sole source of ExitCancelled.
func concludeCancelled(out *Output) int {
	_ = out.Finish()
	_ = out.Close()
	return ExitCancelled
}

// AnyBlockedSoFar reports whether any Task is currently in the Blocked
// state — a live, mid-run check (C12: named "SoFar" to distinguish it from
// Conclusion.AnyBlocked, which reports the finished run's final verdict;
// the two answer different questions and previously shared one name).
func (o *Output) AnyBlockedSoFar() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, t := range o.tasks {
		if t.state == Blocked {
			return true
		}
	}
	return false
}

// AnyFailed reports whether any Task is currently Failed.
func (o *Output) AnyFailed() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, t := range o.tasks {
		if t.state == Failed {
			return true
		}
	}
	return false
}

// AnyBlocked reports whether the finished conclusion is blocked, or any task snapshot is.
func (c Conclusion) AnyBlocked() bool {
	if c.State == StateBlocked {
		return true
	}
	for _, t := range c.Tasks {
		if t.State == Blocked {
			return true
		}
	}
	return false
}
