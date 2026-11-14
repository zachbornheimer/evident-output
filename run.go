package evo

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/present"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
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
	out.hostOwnsSignals = true
	return runInterruptible(ctx, out, run, nil)
}

// Main runs run under SIGINT and SIGTERM and returns the process exit code; it
// never exits. A signal cancels the run's context and ends the run Cancelled with
// exit code 130, including while declared work still runs after run returned.
// Its arguments are Execute's, without the context Main owns.
func Main(cfg Config, run RunFunc) int {
	out := openRun(cfg, tuning{})
	return runSignalled(context.Background(), out, run).ExitCode()
}

// openRun is the one Run constructor: Execute, Main and this package's tests build every Run here. It
// resolves cfg and t once, arms first paint, and captures the workspace relative paths resolve against.
func openRun(cfg Config, t tuning) *Run {
	out := newRun(resolveConfig(cfg, t))
	out.changes.Workspace() // captured at Run start: relative paths never follow a later chdir
	out.arm()
	return out
}

// runSignalled is runInterruptible under SIGINT and SIGTERM: the one place a run subscribes to signals.
func runSignalled(ctx context.Context, out *Run, run RunFunc) Result {
	sigCh := make(chan process.Signal, signalChannelCapacity)
	notifySignals(sigCh, process.Interrupt, process.Terminate)
	defer stopSignals(sigCh)
	return runInterruptible(ctx, out, run, sigCh)
}

// runInterruptible runs run to completion, turning cancellation of ctx or a signal on sigCh into
// cancellation of the active task or the output. A second signal returns ExitCancelled without
// waiting for run to unwind. A nil sigCh never fires: the run watches only ctx.
func runInterruptible(ctx context.Context, out *Run, run RunFunc, sigCh <-chan process.Signal) Result {
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

	cause := cancelCauseUser
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
			cause = context.Cause(ctx).Error()
		}
		break
	}
	interrupted.Store(true)
	out.interruptBecause(cause, string(cancelled))
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
	// Finish already wrote the manifest; this releases its lock and returns any failure. Close never rolls back committed records.
	return r.freshness.Session().Close()
}

// beginRunContext installs ctx as the parent of this run's task scopes, replacing the placeholder from construction.
func (r *Run) beginRunContext(ctx context.Context) context.Context {
	return r.graph.BeginRunContext(ctx)
}

func (r *Run) humanStyle() txt.Style {
	return txt.Style{Color: !r.cfg.noColor, Verbose: r.cfg.verbose, Profile: r.cfg.glyphs}
}

// interruptBecause stops the run at the first interruption, naming cause as the Explanation; graph.Graph.Interrupt owns the order that keeps the ledger honest.
func (r *Run) interruptBecause(cause, reason string) {
	if r == nil {
		return
	}
	r.graph.Interrupt(cause, func() { r.graph.CancelActive(reason) })
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

// subscribeSignals delivers signals on its own channel until release; Main's channel is untouched.
func subscribeSignals(signals []process.Signal) (<-chan process.Signal, func()) {
	c := make(chan process.Signal, signalChannelCapacity)
	notifySignals(c, signals...)
	return c, func() { stopSignals(c) }
}

// signalChannelCapacity holds one signal during cancelActive plus one so a second Ctrl-C is never dropped.
const signalChannelCapacity = 2

// Run is the root of one invocation, handed to the application body by Main and Execute. It is a Container:
// declare root work with Task, Group and Sequence, add run-scoped information with Fact and Summary, ask the
// user with Confirm, and write free text with Text. Evo decides when declared work runs and how it is shown;
// the body only declares it and returns.
type Run struct {
	// mu guards the render state. Its sections hold the run's notifications,
	// so the engine's listener hears a section's writes once the mutex is free.
	mu  record.Mutex
	cfg config

	outputID string
	// startedAt is the wire envelope's started_at (schema/run.v3.json), read back at Finish, never re-derived.
	startedAt time.Time
	closed    bool
	// closing is non-nil once a Close claimed the teardown and closes when it
	// ends, so a concurrent Close waits.
	closing chan struct{}
	// confirmTurn lets one Confirm at a time own the terminal.
	confirmTurn *terminal.Turn
	finishing   bool
	finished    bool
	// misuse records the first API misuse. Graph workers call it without Run.mu;
	// a strict recorder panics after recording, never under graph.mu.
	misuse *misuse.Recorder
	// show presents the run: rows, live region, durable stream, evo.event stream.
	show *present.Presenter

	// changes performs File, Exec, Patch and Effect and owns the workspace paths resolve against.
	changes *change.Run

	tasks []*taskState
	// rootTasks are the tasks outside every collection, in declaration order.
	rootTasks   []*taskState
	collections []*tasksState
	// rec holds the run's truth: journal, ledger, lines, messages, facts, warnings, Conclusion.
	rec *record.Run

	graph *graph.Graph
	// taskStates and containerStates hold engine state per graph node, keyed by node ID.
	taskStates      map[string]*taskState
	containerStates map[string]*tasksState

	// resultText is what ResultWriter received in JSON, whose stdout carries one document the text rides in.
	resultText strings.Builder
	// printLines assembles Print fragments into message lines.
	printLines *txt.PrintLines

	// freshness is this Run's lazily opened reconciliation manifest and
	// what it remembers about each Task.
	freshness  *freshness.Run
	cleanStops graph.CleanStops
	// hostOwnsSignals is true under Execute, where the caller's context is the only way to stop a run.
	hostOwnsSignals bool
}

// runsStarted counts the runs this process has started; it keeps two runs on one frozen Clock apart.
var runsStarted atomic.Uint64

// newRun builds the run cfg describes: its graph, freshness, changes and every projection.
func newRun(cfg config) *Run {
	resolveGlyphProfileLocked(&cfg)
	run := record.NewRun()
	o := &Run{
		cfg:             cfg,
		rec:             run,
		misuse:          misuse.New(),
		confirmTurn:     terminal.NewTurn(),
		taskStates:      make(map[string]*taskState),
		containerStates: make(map[string]*tasksState),
	}
	host := runHost{out: o}
	o.cleanStops.Subscribe = subscribeSignals
	o.graph = graph.New(run, graph.WithMaxEntities(cfg.maxEntities), graph.WithMaxConcurrency(cfg.maxConcurrency), graph.WithMisuseSink(o.misuse), graph.WithHost(host))
	o.freshness = freshness.NewRun(o.graph, freshness.RunConfig{Where: o.manifestConfig, Closed: errClosed, DryRun: host.DryRun, Narration: host})
	o.changes = change.NewRun(o.newChangeHost(), o.graph)
	o.startedAt = cfg.clock.Now()
	o.outputID = record.NewRunID(o.startedAt, process.PID(), runsStarted.Add(1))
	o.show = present.New(present.Config{
		Run: run, Lock: &o.mu, Clock: cfg.clock, Host: presentHost{o},
		RunID: o.outputID, Events: cfg.eventStream(),
		Surface: liveSurfaceOf(cfg), VisibilityDelay: cfg.visibilityDelay, MaxFrameRate: cfg.maxFrameRate,
		Style: o.humanStyle(), Subject: cfg.subject,
		Sinks: humanSinks{o},
	})
	o.printLines = txt.NewPrintLines(o.emitMessageLocked)
	o.mu.Bind(run)
	o.appendEventLocked(recEvent{Type: "output.started", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventRunStarted, "", wire.RunStartedPayload{Name: o.cfg.subject}.EventPayload())
	if cfg.dryRun {
		// Announce before any task can reach the durable stream; unlocked is safe, o has not been returned yet.
		o.show.PlannedHeader()
	}
	return o
}
