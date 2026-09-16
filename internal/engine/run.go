package engine

import (
	"context"
	"errors"
	"os"
	"syscall"
)

// Run executes a CLI presentation lifecycle against this Output and returns
// the Result (Conclusion plus the application error, if any) — the
// Isolated-instance counterpart of Main, for a caller holding its own
// *Output (evo.Init(evo.Config{Isolated: true})).
//
// Typical entrypoint:
//
//	func main() {
//	    out := evo.Init(evo.Config{Title: "tool", Isolated: true})
//	    os.Exit(out.Run(context.Background(), run).ExitCode())
//	}
//
// ctx carries caller-driven cancellation into run in addition to the
// SIGINT/SIGTERM wiring below; a nil ctx runs as context.Background().
//
// Lifecycle: arm first paint → run → (reconcile run error into model) →
// Finish → Close.
//
// Result.Conclusion.ExitCode:
//   - nil Output → ExitFailed (2)
//   - SIGINT/SIGTERM → Cancel on the active task (or the output) → ExitCancelled (130)
//   - a second SIGINT/SIGTERM → ExitCancelled (130) returned immediately, without
//     waiting for run to unwind, so the caller's
//     os.Exit(out.Run(...).ExitCode()) exits now
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
// so the human conclusion cannot show [ready] while the process fails, and is
// also returned as Result.Err so embedders can distinguish work failure from
// presentation/transport failure.
func (o *Output) Run(ctx context.Context, run RunFunc) Result {
	if o == nil {
		return Result{Conclusion: Conclusion{State: StateFailed, ExitCode: ExitFailed}}
	}
	o.arm()
	if ctx == nil {
		ctx = context.Background()
	}
	return runInterruptible(ctx, o, run)
}

// Run executes a CLI presentation lifecycle against the package-level default
// instance (see Init) and returns the Result, never exiting — the
// package-level counterpart of Output.Run, for callers (tests, or a caller
// composing its own exit path) that need the Result without Main's exit-code
// derivation.
//
// run reports only an error; the Conclusion (0/1/2/130) is the sole source of
// the exit code — see Output.Run for the full lifecycle and signal contract.
func Run(ctx context.Context, run RunFunc) Result {
	out := Default()
	out.arm()
	if ctx == nil {
		ctx = context.Background()
	}
	return runInterruptible(ctx, out, run)
}

// Main runs a CLI presentation lifecycle against the package-level default
// instance (see Init) and returns the derived exit code; it does not itself
// call os.Exit — the library still never calls os.Exit directly (aPI-018),
// so the caller owns process termination:
//
//	func main() {
//	    evo.Init(evo.Config{Title: "tool"})
//	    os.Exit(evo.Main(run))
//	}
//
// Callers that need the full Result (Conclusion plus application error, for
// embedding or JSON projection) call Run instead.
func Main(run RunFunc) int {
	return Run(context.Background(), run).ExitCode()
}

// runInterruptible executes run to completion, wiring SIGINT/SIGTERM into
// cancellation of the active task (or the output itself) — and of the
// derived ctx passed to run — so the ledger and exit code always agree. A
// second signal returns ExitCancelled immediately instead of waiting for run
// to unwind — the process-level os.Exit that wraps a caller's
// os.Exit(evo.Main(run)) is what actually terminates.
func runInterruptible(ctx context.Context, out *Output, run RunFunc) Result {
	sigCh := make(chan os.Signal, signalChannelCapacity)
	notifySignals(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals(sigCh)

	// runCtx becomes o.Context() for the duration of this run (see
	// beginRunContext): every Define/Verify task scope started from here
	// on descends from the caller's own ctx, not just from run's local
	// parameter.
	runCtx := out.beginRunContext(ctx)

	done := make(chan error, 1)
	go func() {
		var err error
		if run != nil {
			err = run(runCtx)
		}
		done <- err
	}()

	select {
	case runErr := <-done:
		return concludeRun(out, runErr)
	case <-sigCh:
		// out.interrupt cancels o.cancelRun, the same cancel beginRunContext
		// installed above — no separate local cancel is needed.
		out.interrupt("interrupted")
		select {
		case runErr := <-done:
			return concludeCancelled(out, runErr)
		case <-sigCh:
			return Result{Conclusion: Conclusion{State: StateCancelled, Cancelled: true, ExitCode: ExitCancelled}}
		}
	}
}

// concludeRun reconciles an ordinary (non-interrupted) run outcome into the
// Result.
func concludeRun(out *Output, runErr error) Result {
	if runErr != nil && !out.anyFailed() {
		// Synchronize the presentation model with the application error only when
		// no entity already recorded Failed — avoids a duplicate synthetic Fail row
		// on top of an existing task/item Fail. Exit code still comes from conclusion.
		out.Fail(runErr.Error())
	}
	finishErr := out.Finish()
	closeErr := out.Close()
	conclusion := out.Conclusion()
	// Bookkeeping misuse (a leftover unresolved task, a duplicate key, ...)
	// is already folded into the Conclusion itself before the band renders
	// (release-gate finding 2) — ExitCode already agrees with what printed.
	// A genuine renderer/write failure is different: it surfaces only after
	// the band is already flushed, so the band never had a chance to
	// reflect it — that still escalates an otherwise-OK exit code here.
	if conclusion.ExitCode == ExitOK && (errors.Is(finishErr, ErrRenderer) || errors.Is(closeErr, ErrRenderer)) {
		conclusion.ExitCode = ExitFailed
	}
	return Result{Conclusion: conclusion, Err: runErr}
}

// concludeCancelled finalizes an interrupted run; the Conclusion computed
// from the Cancel already recorded is the sole source of ExitCancelled.
// runErr is the (usually context.Canceled-flavored) error run returned after
// cancellation, carried through as Result.Err for embedders.
func concludeCancelled(out *Output, runErr error) Result {
	_ = out.Finish()
	_ = out.Close()
	return Result{Conclusion: out.Conclusion(), Err: runErr}
}

// anyBlockedSoFar reports whether any Task is currently in the Blocked
// state — a live, mid-run check (C12: named "SoFar" to distinguish it from
// Conclusion.AnyBlocked, which reports the finished run's final verdict;
// the two answer different questions and previously shared one name).
// Internal only (P6 deletion census: no consumer needed the mid-run
// question once Run/Conclusion existed) — see anyblocked_package_test.go.
func (o *Output) anyBlockedSoFar() bool {
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

// anyFailed reports whether any Task is currently Failed.
func (o *Output) anyFailed() bool {
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
