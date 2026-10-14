package evo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
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

	// runCtx is o.Context() for this run, detached from the caller's cancellation: the watcher
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
	out.interrupt("interrupted")
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

// resultErr is Result.Err: what the body returned, joined with the first misuse the run recorded.
func (o *Run) resultErr(runErr error) error {
	misused := o.firstMisuse()
	switch {
	case misused == nil:
		return runErr
	case runErr == nil:
		return misused
	default:
		return errors.Join(runErr, misused)
	}
}

func (o *Run) anyFailed() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, t := range o.tasks {
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
func (o *Run) selfName() string {
	return o.cfg.subject
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

func (o *Run) ensureOpen() error {
	if o.closed || o.finishing || o.finished {
		return errClosed
	}
	return nil
}

// close is idempotent with a best-effort Finish: the first call tears down; a concurrent or later call waits and returns nil.
func (o *Run) close() error {
	o.mu.Lock()
	if o.closing != nil {
		closing := o.closing
		o.mu.Unlock()
		<-closing
		return nil
	}
	o.closing = make(chan struct{})
	defer close(o.closing)
	// A Confirm still waiting would hold its caller forever once the run is gone.
	for o.graph.CancelPendingGate(cancelCauseClose) {
	}
	needFinish := !o.finished
	o.mu.Unlock()
	if needFinish {
		_ = o.finish()
	}
	o.mu.Lock()
	o.show.Live.Stop()
	o.show.Stream.StopHeartbeats()
	o.closed = true
	o.graph.Close()
	o.mu.Unlock()
	o.graph.Cancel()
	// Finish already wrote the manifest; this releases its lock (spec §11.3) and returns any failure. Close never rolls back committed records.
	return o.freshness.Session().Close()
}

// beginRunContext installs ctx as the parent of this run's task scopes, replacing the placeholder from construction.
func (o *Run) beginRunContext(ctx context.Context) context.Context {
	return o.graph.BeginRunContext(ctx)
}

func (o *Run) conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok := o.rec.Conclusion(); ok {
		return asEvoConclusion(c)
	}
	snap := o.snapshotLocked()
	c := record.InferConclusion(snap)
	o.explainCancellationLocked(&c)
	return asEvoConclusion(c)
}

const cancelCauseUser = "by user"

const cancelCauseClose = "run closed"

// explainCancellationLocked names what interrupted a cancelled run as its Explanation.
func (o *Run) explainCancellationLocked(c *record.Conclusion) {
	if c.Outcome != record.OutcomeCancelled {
		return
	}
	if cause := o.graph.CancelCause(); cause != "" {
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
	incomplete = record.Incomplete
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

func (o *Run) humanStyle() txt.Style {
	return txt.Style{Color: !o.cfg.noColor, Verbose: o.cfg.verbose, Profile: o.cfg.glyphs}
}

func (o *Run) recordMisuse(err error) {
	o.misuse.Record(err)
}

func (o *Run) recordMisuseFor(subject string, err error) {
	o.misuse.RecordMisuseFor(subject, err)
}

func (o *Run) firstMisuse() error {
	return o.misuse.First()
}

// appendMisuseLineLocked renders the one required line for the first recorded misuse, so an exit code
// never disagrees with what was printed. It is the corrective sentence; machine detail stays in JSON/debug.
func (o *Run) appendMisuseLineLocked() {
	o.graph.Root().Log(txt.MisuseLine(o.misuse.Hint(), !o.cfg.noColor))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second terminal verb on task name.
func (o *Run) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	o.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}

// redactString applies the configured redactor. It takes no lock: locking Output.mu here deadlocked a
// scheduled failure auto-attaching its evidence tail, where resolve already holds it.
func (o *Run) redactString(s string) string {
	if o == nil {
		return s
	}
	r := o.cfg.redactor
	if r == nil {
		return s
	}
	return r.RedactString(s)
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

// warnLocked records p as a run-scoped warning and renders it; callers hold o.mu.
func (o *Run) warnLocked(p problem) {
	o.graph.Root().Warn(p)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "run.warned", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(o.cfg.glyphs), txt.SGRYellow, !o.cfg.noColor)
	o.show.Stream.Write(glyph + " " + p.Summary + "\n")
}

// interrupt stops the run at the first signal; graph.Graph.Interrupt owns the order that keeps the ledger honest.
func (o *Run) interrupt(reason string) {
	if o == nil {
		return
	}
	o.graph.Interrupt(cancelCauseUser, func() { o.graph.CancelActive(reason) })
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

// Verify registers a read-only check of the world's current state, ANDed with any earlier Verify on this Task.
// Call it before Define, which freezes configuration; a later call is ignored and recorded as misuse.
//
// Evo runs every Verify twice. Before the callback, all true skips it and settles the Task Satisfied, any
// false runs it, and an error fails the Task. After a callback that returned nil, any false fails the Task,
// because the work did not produce the state it promised. The second check is skipped when the callback
// ended the Task with an error, or when a dry run planned its changes without making them.
func (t *Task) Verify(fn func(context.Context) (bool, error)) *Task {
	if t == nil || t.out == nil || fn == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	if st.node.Submitted() {
		o.recordMisuseFor(st.name, ErrInvalidConfig)
		return t
	}
	o.freshness.Tasks().AddVerifier(st.node.TaskID(), freshness.Verifier(fn))
	return t
}

// Define freezes this Task's After and Verify configuration and submits fn to the scheduler. It returns the
// same *Task at once and never runs fn inline; Evo decides when fn runs and owns its goroutine, so callers
// declare work and return. What fn returns is the Task's outcome: nil Succeeded, Refuse Refused, Exclude
// Excluded, and Fail or any other error Failed. A second Define on the same Task is misuse.
func (t *Task) Define(fn func(context.Context) error, opts ...DefineOption) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	if fn == nil {
		o.recordMisuse(ErrInvalidConfig)
		return t
	}
	st := t.declared()
	if st == nil {
		o.recordMisuse(t.rejected)
		return t
	}
	var cfg defineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	graph.Define(o.graph, st.node, o.definition(st, o.cleanStops.Wrap(cfg.cleanStopSignals, fn)))
	return t
}

// definition is st's Define with fn as its callback; freshness checks it.
func (o *Run) definition(st *taskState, fn func(context.Context) error) graph.Definition[*change.Run] {
	return graph.Definition[*change.Run]{Owner: o.changes, Checks: o.freshness.Checks(st.node.TaskID()), Callback: fn}
}
