package engine

import (
	"context"
	"errors"
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
// A nil ctx runs as context.Background(). What the end of ctx means
// depends on the Output (DEC-CANCEL-005):
//   - an Embedded (Config.Embedded) Output treats ctx as its request
//     lifecycle. Tasks never see ctx's cancellation or deadline directly;
//     when ctx ends, the run is interrupted exactly as ^C interrupts a CLI
//     (running Tasks cancelled, queued Tasks not_started). Such a run
//     registers no SIGINT/SIGTERM handler — its host owns process signals.
//   - every other Output passes ctx to its Tasks unchanged: a Define that
//     returns ctx.Err() fails its row, and the run concludes failed.
//
// Lifecycle: arm first paint → run → (reconcile run error into model) →
// Finish → Close. An Embedded Output's ctx, and SIGINT/SIGTERM on every
// CLI format, are watched until the run concludes, not only until run
// returns. A FormatExternal Output that is not Embedded keeps the 1.1
// signal window: a signal is acted on only while run runs, and one that
// arrives after run returned is caught and ignored (DEC-CANCEL-005).
//
// Result.Conclusion.ExitCode:
//   - nil Output → ExitFailed (2)
//   - SIGINT/SIGTERM (non-embedded only, within the signal window above)
//     → Cancel on the active task (or the output) → ExitCancelled (130),
//     cause "by user"
//   - embedded Output whose ctx ends before its work finishes →
//     ExitCancelled (130), cause "by caller" or "deadline exceeded"; a ctx
//     that ends after the callback returned and every Task finished leaves
//     the verdict unchanged (DEC-CANCEL-004)
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

// runInterruptible executes run to completion, turning SIGINT/SIGTERM
// (non-embedded Outputs) and the end of an embedded Output's caller ctx
// into one ordered interrupt of the active task (or the output itself) —
// and of the run context passed to run — so the ledger and exit code
// always agree. The caller's ctx, and signals on every CLI format, are
// watched until the run concludes, not just until run returns: the
// ordinary shape declares Tasks and returns, and their Define work
// executes while Finish waits. A FormatExternal run that is not Embedded
// keeps the 1.1 window instead (signalsUntilCallbackReturns). A second
// signal returns ExitCancelled immediately instead of waiting for the work
// to unwind.
func runInterruptible(ctx context.Context, out *Output, run RunFunc) Result {
	// runCtx becomes o.Context() for the duration of this run (see
	// beginRunContext). It is installed before either watch starts, so the
	// first interrupt — even one a pre-cancelled ctx fires at once —
	// cancels this context rather than a placeholder it would then replace
	// (DEC-CANCEL-003).
	runCtx := out.beginRunContext(out.scopeCaller(ctx))
	signals := out.subscribeProcessSignals()
	defer signals.stop()
	caller := out.watchCaller(ctx)
	defer caller.release()

	phase := new(runPhase)
	results := make(chan Result, 1)
	go func() {
		var runErr error
		if run != nil {
			runErr = run(runCtx)
		}
		out.endRunCallback(caller.ended)
		if phase.callbackReturned() || caller.interruptIfEnded() {
			results <- concludeCancelled(out, runErr)
			return
		}
		results <- concludeRun(out, runErr)
	}()

	if result, concluded := signals.awaitInterrupt(results, phase); concluded {
		return result
	}
	// out.interrupt cancels o.cancelRun, the same cancel beginRunContext
	// installed above — no separate local cancel is needed.
	out.interrupt(interruptionBySignal)
	select {
	case result := <-results:
		return result
	case <-signals.received:
		return Result{Conclusion: Conclusion{State: StateCancelled, Cancelled: true, ExitCode: ExitCancelled}}
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
