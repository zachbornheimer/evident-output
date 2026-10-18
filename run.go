package evo

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
)

var errRunExitedWithoutReturn = errors.New("run callback exited without returning (runtime.Goexit)")

// Execute runs run to completion and returns its Result. It installs no signal
// handlers and never exits the process: the caller owns the context and the exit,
// which makes Execute the entry point for embedding and for tests. A nil ctx is
// context.Background.
//
// ctx governs the whole run, including declared work still running after run
// returned. Once ctx is done, every running callback's context is done and the
// run ends Cancelled with exit code 130. Use Main for a program that should end
// on SIGINT or SIGTERM.
func Execute(ctx context.Context, cfg Config, run RunFunc) Result {
	out := openRun(cfg, tuning{})
	if ctx == nil {
		ctx = context.Background()
	}
	return runInterruptible(ctx, out, run)
}

// Main runs run under SIGINT and SIGTERM and returns the process exit code; it
// never exits. A signal cancels the run's context and ends the run Cancelled with
// exit code 130, including while declared work still runs after run returned.
// Its arguments are Execute's, without the context Main owns.
func Main(cfg Config, run RunFunc) int {
	out := openRun(cfg, tuning{})
	return runInterruptible(context.Background(), out, run).ExitCode()
}

// openRun is the one Run constructor: Execute, Main and this package's tests build every Run here. It
// resolves cfg and t once, arms first paint, and captures the workspace relative paths resolve against.
func openRun(cfg Config, t tuning) *Run {
	out := newRun(resolveConfig(cfg, t))
	out.changes.Workspace() // captured at Run start: relative paths never follow a later chdir
	out.arm()
	return out
}

// runInterruptible runs run to completion, turning SIGINT/SIGTERM into cancellation of the
// active task or the output. A second signal returns ExitCancelled without waiting for run to unwind.
func runInterruptible(ctx context.Context, out *Run, run RunFunc) Result {
	sigCh := make(chan process.Signal, signalChannelCapacity)
	notifySignals(sigCh, process.Interrupt, process.Terminate)
	defer stopSignals(sigCh)

	// runCtx is r.Context() for this run, detached from the caller's cancellation: the watcher
	// records the Cancel first, so a task that sees Done ends Cancelled, not Failed with a deadline error.
	runCtx := out.beginRunContext(watchedContext{ctx})

	// One goroutine runs the body and the Finish drain so the signal watcher stays armed until the Result exists.
	var interrupted atomic.Bool
	done := make(chan Result, 1)
	go func() {
		// Sent from a defer so a callback ending via runtime.Goexit (testing.T.FailNow) still releases the waiter.
		var (
			err      error
			returned bool
			result   Result
			finished bool
		)
		defer func() {
			if r := recover(); r != nil {
				// The panic ends the process; the run records it as the panic it was, not as an exit.
				concludeRunAfterExit(out, &interrupted, true, graph.PanicError(r))
				panic(r)
			}
			if !finished {
				result = concludeRunAfterExit(out, &interrupted, returned, err)
			}
			done <- result
		}()
		if run != nil {
			err = run(runCtx, out)
		}
		returned = true
		result = concludeInterruptible(out, &interrupted, err)
		finished = true
	}()

	for {
		select {
		case result := <-done:
			return result
		case sig := <-sigCh:
			// A signal a running Task named in CleanStop ends only that Task.
			if out.cleanStops.Stop(sig) {
				continue
			}
		case <-ctx.Done():
		}
		break
	}
	interrupted.Store(true)
	out.interrupt(string(cancelled))
	return awaitInterrupted(sigCh, done)
}

// awaitInterrupted waits for the Result; a second signal returns Cancelled at once.
func awaitInterrupted(sigCh <-chan process.Signal, done <-chan Result) Result {
	select {
	case result := <-done:
		return result
	case <-sigCh:
		return Result{Conclusion: Conclusion{Outcome: Cancelled, cancelled: true, exitCode: exitCancelled}}
	}
}

// watchedContext keeps the caller's deadline and values but not its cancellation.
type watchedContext struct{ parent context.Context }

func (w watchedContext) Deadline() (time.Time, bool) { return w.parent.Deadline() }

func (w watchedContext) Done() <-chan struct{} { return nil }

func (w watchedContext) Err() error { return nil }

func (w watchedContext) Value(key any) any { return w.parent.Value(key) }

func (r *Run) ensureOpen() error {
	if r.closed || r.finishing || r.finished {
		return errClosed
	}
	return nil
}

// close is idempotent with a best-effort Finish: the first call tears down; a concurrent or later call waits and returns nil.
func (r *Run) close() error {
	r.mu.Lock()
	if r.closing != nil {
		closing := r.closing
		r.mu.Unlock()
		<-closing
		return nil
	}
	r.closing = make(chan struct{})
	defer close(r.closing)
	// A Confirm still waiting would hold its caller forever once the run is gone.
	for r.graph.CancelPendingGate(cancelCauseClose) {
	}
	needFinish := !r.finished
	r.mu.Unlock()
	if needFinish {
		_ = r.finish()
	}
	r.mu.Lock()
	r.show.Live.Stop()
	r.show.Stream.StopHeartbeats()
	r.closed = true
	r.graph.Close()
	r.mu.Unlock()
	r.graph.Cancel()
	// Finish already wrote the manifest; this releases its lock (spec §11.3) and returns any failure. Close never rolls back committed records.
	return r.freshness.Session().Close()
}

// beginRunContext installs ctx as the parent of this run's task scopes, replacing the placeholder from construction.
func (r *Run) beginRunContext(ctx context.Context) context.Context {
	return r.graph.BeginRunContext(ctx)
}

func (r *Run) humanStyle() txt.Style {
	return txt.Style{Color: !r.cfg.noColor, Verbose: r.cfg.verbose, Profile: r.cfg.glyphs}
}

// interrupt stops the run at the first signal; graph.Graph.Interrupt owns the order that keeps the ledger honest.
func (r *Run) interrupt(reason string) {
	if r == nil {
		return
	}
	r.graph.Interrupt(cancelCauseUser, func() { r.graph.CancelActive(reason) })
}

// RunFunc is the shape of application work; its context carries SIGINT/SIGTERM cancellation.
type RunFunc func(context.Context, *Run) error

// signalNotifier and signalStopper abstract signal delivery (facade rule) so tests need no real signals.
type (
	signalNotifier func(c chan<- process.Signal, sig ...process.Signal)
	signalStopper  func(c chan<- process.Signal)
)

var (
	notifySignals signalNotifier = process.Notify
	stopSignals   signalStopper  = process.StopNotify
)

// signalChannelCapacity holds one signal during cancelActive plus one so a second Ctrl-C is never dropped.
const signalChannelCapacity = 2
