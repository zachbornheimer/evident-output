package evo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
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
func (w watchedContext) Done() <-chan struct{}       { return nil }
func (w watchedContext) Err() error                  { return nil }
func (w watchedContext) Value(key any) any           { return w.parent.Value(key) }

func concludeInterruptible(out *Run, interrupted *atomic.Bool, runErr error) Result {
	if interrupted.Load() {
		return concludeCancelled(out, runErr)
	}
	return concludeRun(out, runErr)
}

func concludeRunAfterExit(out *Run, interrupted *atomic.Bool, returned bool, runErr error) Result {
	if !returned {
		runErr = errRunExitedWithoutReturn
	}
	return concludeInterruptible(out, interrupted, runErr)
}

func concludeRun(out *Run, runErr error) Result {
	if runErr != nil && !out.anyFailed() {
		// Record the application error only when no entity already Failed (avoids a duplicate Fail row).
		out.end(runErr)
	}
	finishErr := out.finish()
	closeErr := out.close()
	conclusion := out.conclusion()
	// Bookkeeping misuse is already in the Conclusion. A renderer or write failure surfaces after
	// the band is flushed, so it escalates an otherwise-OK exit code here.
	if conclusion.exitCode == exitOK && (errors.Is(finishErr, errRenderer) || errors.Is(closeErr, errRenderer)) {
		conclusion.exitCode = exitFailed
	}
	return Result{Conclusion: conclusion, Err: out.resultErr(runErr)}
}

// concludeCancelled: the recorded Cancel's Conclusion alone yields ExitCancelled; runErr rides as Result.Err.
func concludeCancelled(out *Run, runErr error) Result {
	_ = out.finish()
	_ = out.close()
	return Result{Conclusion: out.conclusion(), Err: out.resultErr(runErr)}
}

// resultErr is Result.Err: what the body returned, joined with every misuse the run recorded.
func (r *Run) resultErr(runErr error) error {
	return joinErrors(runErr, r.allMisuse())
}

func (r *Run) anyFailed() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.node.Rec.State() == failed {
			return true
		}
	}
	return false
}

// processArgv0 is the facade over the program name; a var so tests can inject it.
var processArgv0 = func() string {
	args := process.Args()
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// selfName is the resolved Config.Name: the binary's basename when none was given.
func (r *Run) selfName() string {
	return r.cfg.subject
}

// identityFallbackName is the executable's basename or "command"; never a Subject.
func identityFallbackName() string {
	if base := filepath.Base(processArgv0()); base != "." && base != string(filepath.Separator) {
		return base
	}
	return "command"
}

// rendererError reports a renderer failure as a genuine Run failure (§32.2); nil stays nil.
func rendererError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", errRenderer, err)
}

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

func (r *Run) conclusion() Conclusion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.rec.Conclusion(); ok {
		return asEvoConclusion(c)
	}
	snap := r.snapshotLocked()
	c := record.InferConclusion(snap)
	r.explainCancellationLocked(&c)
	return asEvoConclusion(c)
}

const cancelCauseUser = "by user"

const cancelCauseClose = "run closed"

// explainCancellationLocked names what interrupted a cancelled run as its Explanation.
func (r *Run) explainCancellationLocked(c *record.Conclusion) {
	if c.Outcome != record.OutcomeCancelled {
		return
	}
	if cause := r.graph.CancelCause(); cause != "" {
		c.Explanation = cause
	}
}

// Domain types live in internal/core and internal/txt; engine aliases them.

type entityState = record.State

const (
	pending    = record.Pending
	running    = record.Running
	succeeded  = record.Succeeded
	refused    = record.Refused
	failed     = record.Failed
	excluded   = record.Excluded
	cancelled  = record.Cancelled
	empty      = record.Empty
	notStarted = record.NotStarted
)

type progressKind = record.ProgressKind

const (
	indeterminate = record.Indeterminate
	determinate   = record.Determinate
	bytesKind     = record.BytesKind
)

type (
	factRecord = record.Fact
	recEvent   = record.Event
)

const eventSchemaVersion = record.EventSchemaVersion

type (
	snapshot        = record.Snapshot
	taskSnapshot    = record.TaskSnapshot
	tasksSnapshot   = record.TasksSnapshot
	changesSnapshot = record.ChangesSnapshot
	planSnapshot    = record.PlanSnapshot
)

const (
	exitOK        = record.ExitOK
	exitRefused   = record.ExitRefused
	exitFailed    = record.ExitFailed
	exitCancelled = record.ExitCancelled
)

type (
	problem      = record.Problem
	field        = record.Field
	commandSpec  = record.CommandSpec
	glyphProfile = txt.GlyphProfile
)

const (
	glyphsAuto    = txt.GlyphsAuto
	glyphsUnicode = txt.GlyphsUnicode
	glyphsASCII   = txt.GlyphsASCII
)

func (r *Run) humanStyle() txt.Style {
	return txt.Style{Color: !r.cfg.noColor, Verbose: r.cfg.verbose, Profile: r.cfg.glyphs}
}

func (r *Run) recordMisuse(err error) {
	r.misuse.Record(err)
}

func (r *Run) recordMisuseFor(subject string, err error) {
	r.misuse.RecordMisuseFor(subject, err)
}

func (r *Run) firstMisuse() error {
	return r.misuse.First()
}

// allMisuse is every distinct misuse recorded, joined in record order.
func (r *Run) allMisuse() error {
	return r.misuse.All()
}

// appendMisuseLineLocked renders the one required line for the first recorded misuse, so an exit code
// never disagrees with what was printed. It is the corrective sentence; machine detail stays in JSON/debug.
func (r *Run) appendMisuseLineLocked() {
	r.graph.Root().Log(txt.MisuseLine(r.misuse.Hint(), !r.cfg.noColor))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second terminal verb on task name.
func (r *Run) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	r.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}

// redactString applies the configured redactor. It takes no lock: locking Run.mu here deadlocked a
// scheduled failure auto-attaching its evidence tail, where resolve already holds it.
func (r *Run) redactString(s string) string {
	if r == nil {
		return s
	}
	redactor := r.cfg.redactor
	if redactor == nil {
		return s
	}
	return redactor.RedactString(s)
}

// Fact accumulates a run-scoped name/value annotation (like Task.Fact): it renders a durable dim line
// at once and stores it for Snapshot/JSON. A nil Run records nothing.
func (r *Run) Fact(name, value string) *Run {
	if r == nil {
		return r
	}
	f := record.SanitizeFact(factRecord{Name: txt.Text(name), Value: txt.Text(value)})
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpen(); err != nil {
		r.recordMisuse(err)
		return r
	}
	r.graph.Root().Note(f)
	r.bumpLocked()
	r.emitWireEventLocked(wire.EventFactRecorded, "", wire.ToFactDoc(f).EventPayload())
	r.show.Stream.Write(txt.Dim(f.Name+"  "+f.Value, !r.cfg.noColor) + "\n")
	return r
}

// warnLocked records p as a run-scoped warning and renders it; callers hold r.mu.
func (r *Run) warnLocked(p problem) {
	r.graph.Root().Warn(p)
	r.bumpLocked()
	r.appendEventLocked(recEvent{Type: "run.warned", OutputID: r.outputID})
	r.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(r.cfg.glyphs), txt.SGRYellow, !r.cfg.noColor)
	r.show.Stream.Write(glyph + " " + p.Summary + "\n")
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
