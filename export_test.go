package evo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/live"
	renderplain "github.com/zachbornheimer/evident-output/v2/internal/project/plain"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// Aliases so evo_test keeps calling the ForTest names the public tests use.

type (
	TestClock       = clock.Fixed
	TestSystemClock = systemClock_1
	TestRedactor    = noopRedactor_1
	TestCapture     = capture
)

func DelayForTest(d time.Duration) *time.Duration { return delay(d) }

func KeepLastLinesForTest(n int) captureOption  { return keepLastLines_1(n) }
func MirrorToDiagnosticsForTest() captureOption { return mirrorToDiagnostics_1() }
func MirrorToDebugForTest() captureOption       { return mirrorToDebug_1() }

// Hooks only tests reach: observation, drivers and aliases moved out of
// production so the shipped package carries no test-only code.

// Run executes a CLI presentation lifecycle against this Output and returns
// the Result (Conclusion plus the application error, if any) — the
// Isolated-instance counterpart of Main, for a caller holding its own
// *Run ((*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error { return nil })).
//
// Typical entrypoint:
//
//	func main() {
//	    out := (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: "tool"}, func(ctx context.Context, r *evo.Run) error { return nil })
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
// so the human conclusion cannot read as success while the process fails, and is
// also returned as Result.Err so embedders can distinguish work failure from
// presentation/transport failure.
func (o *Run) run(ctx context.Context, run RunFunc) Result {
	if o == nil {
		return Result{Conclusion: Conclusion{Outcome: Failed, exitCode: exitFailed}}
	}
	o.arm()
	if ctx == nil {
		ctx = context.Background()
	}
	return runInterruptible(ctx, o, run)
}

// initRun builds a Run through the one constructor Execute and Main use, with t's settings when given.
func initRun(cfg Config, t ...tuning) *Run {
	var tune tuning
	if len(t) > 0 {
		tune = t[0]
	}
	return openRun(cfg, tune)
}

// SwapLookupEnv replaces the process-environment facade. Tests inject a
// map instead of racing os.Setenv. Restore via the returned function.
func swapLookupEnv(fn func(string) string) func() {
	lookupEnvMu.Lock()
	prev := lookupEnvFn
	lookupEnvFn = fn
	lookupEnvMu.Unlock()
	return func() {
		lookupEnvMu.Lock()
		lookupEnvFn = prev
		lookupEnvMu.Unlock()
	}
}

func keepLastLines_1(n int) captureOption { return keepLastLines(n) }

func mirrorToDiagnostics_1() captureOption { return mirrorToDiagnostics() }

func mirrorToDebug_1() captureOption { return mirrorToDebug() }

func renderPlain_1(s snapshot, opts plainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}

func fSPath(path string) fingerprint { return freshness.FSPath(path) }

func (t *Task) captureForTest(opts ...captureOption) *record.Capture {
	return t.capture(opts...)
}

func (o *Run) events() []recEvent { return o.copyEvents() }

func (o *Run) debugForTest(message string, fields ...field) {
	o.debug(message, fields...)
}

func (o *Run) alsoWriteForTest(w io.Writer) {
	if w == nil {
		return
	}
	o.cfg.extraWriters = append(o.cfg.extraWriters, w)
}

func (o *Run) dropDiagnosticForTest() { o.cfg.diagnostic = nil }

func (o *Run) forceLiveVisibleForTest() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.show.Live.Show()
}

func (o *Run) debugWriterForTest() io.WriteCloser { return o.debugWriter() }

func (o *Run) subjectForTest(text string) { o.subject(text) }

func (o *Run) slogHandlerForTest() slog.Handler { return o.slogHandler() }

func (o *Run) atForTest(visibility visibility) *printer { return o.at(visibility) }

func (o *Run) schedulerMaxObserved() int {
	return o.graph.MaxObserved()
}

// Value fingerprints a caller-supplied scalar (string, bool, any signed/
// unsigned integer, any finite float, time.Time, or []byte) under a stable,
// safe name. Only the digest and name are ever persisted, never the raw
// value. Any other type is an error when the Fingerprint is taken.
func value(name string, v any) fingerprint { return freshness.Value(name, v) }

// App fingerprints the running application itself: the executable's bytes
// when readable, else its Go build ID, else an error when the Fingerprint
// is taken. Include it in a Task's Basis only when the application's own
// implementation is a semantic input to that Task's result.
func app() fingerprint { return freshness.App() }

// colorLevel describes terminal color support. Unexported (C8): no public
// entry point ever returned this to a caller except detectCapabilities,
// which is unexported alongside it.
type colorLevel int

const (
	colorNone colorLevel = iota
	colorBasic
	color256
	colorTrue
)

// capabilityProfile holds terminal capability facts (§22). Unexported (C8).
type capabilityProfile struct {
	Interactive bool
	Color       colorLevel
	Unicode     bool
	Width       int
	Height      int
	NoColor     bool
}

// detectCapabilities builds a profile from options and environment-like
// hints. Unexported (C8: no external caller). It does not read the real
// environment in the core package without injection; callers pass
// NoColor/Width/Plain options instead.
func detectCapabilities(opts ...option) capabilityProfile {
	var c Config
	var t tuning
	for _, o := range opts {
		if o != nil {
			o(&c, &t)
		}
	}
	cfg := resolveConfig(c, t)
	p := capabilityProfile{
		Interactive: !cfg.plain,
		Unicode:     true,
		Width:       cfg.width,
		Height:      24,
		NoColor:     cfg.noColor,
		Color:       colorBasic,
	}
	// Plain/non-interactive still allow semantic color on final reports unless NoColor.
	if cfg.noColor {
		p.Color = colorNone
	}
	if cfg.terminal != nil {
		if ls := live.SurfaceOf(cfg.terminal); ls != nil {
			p.Width = ls.Columns()
			p.Height = ls.Rows()
			p.Interactive = ls.IsInteractive() && p.Interactive
		}
	}
	return p
}

// Pluralize returns the plural spelling of singular when quantity != 1 (an
// irregular table for the common exceptions, else the regular English
// +s/+es/+ies rule), and singular unchanged when quantity == 1 — the
// object-pluralization counterpart to the ledger's verb tense, so a
// mutation call site stops writing its own singular/plural noun() switch:
//
//	worktrees.Remove(n, evo.Pluralize(n, "worktree")) // "1 worktree" / "2 worktrees"
//
// A glob/path/symbol object ("stale origin/*", "*.tmp") renders unchanged at
// any quantity instead of blindly gaining a trailing "s" ("stale origin/*s")
// the +s/+es/+ies rule was never designed to produce.
func pluralize(quantity int64, singular string) string {
	return project.Pluralize(quantity, singular)
}

func keepLastLines(n int) captureOption {
	return func(_ *Run, _ string, cfg *record.CaptureConfig) {
		if n > 0 {
			cfg.MaxLines = n
		}
	}
}

func mirrorToDiagnostics() captureOption { return mirrorCapture(true, false) }

func mirrorToDebug() captureOption { return mirrorCapture(false, true) }

// mirrorCapture also writes each kept line to Diagnostics, to the debug
// journal, or both.
func mirrorCapture(diag, debug bool) captureOption {
	return func(o *Run, taskName string, cfg *record.CaptureConfig) {
		if o == nil {
			return
		}
		prior := cfg.OnLine
		cfg.OnLine = func(line string) {
			if prior != nil {
				prior(line)
			}
			o.mirrorCaptureLine(diag, debug, taskName, line)
		}
	}
}

func (o *Run) mirrorCaptureLine(mirrorDiag, mirrorDebug bool, taskName, line string) {
	if o == nil {
		return
	}
	if mirrorDiag {
		o.writeDiagnosticText(line + "\n")
	}
	if !mirrorDebug {
		return
	}
	o.mu.Lock()
	allowDebug := o.cfg.debugLevel <= levelDebug
	hasDiag := o.cfg.diagnostic != nil
	interactive := o.show.Live.Interactive()
	o.mu.Unlock()
	if !allowDebug {
		return
	}
	if !hasDiag && !interactive {
		return
	}
	if taskName != "" {
		o.debug(line, field{Key: "task", Value: taskName})
	} else {
		o.debug(line)
	}
}

// Err returns the first recorded misuse error, if any.
func (o *Run) err() error {
	return o.firstMisuse()
}

// noopRedactor leaves strings unchanged.
type noopRedactor = process.NoopRedactor

func asRecordResult(r Result) record.Result {
	return record.Result{Conclusion: asRecordConclusion(r.Conclusion), Err: r.Err}
}

type (
	effectRecord = record.EffectRecord

	sourceLocation = record.SourceLocation

	attachment = record.Attachment

	capture = record.Capture

	systemClock_1 = systemClock

	noopRedactor_1 = noopRedactor

	// Fingerprint observes one Basis input's current content identity
	// (spec §11.1). Implementations must not mutate the world.
	fingerprint = freshness.Fingerprint
)

// DebugWriter returns a line-oriented writer that emits Debug lines on newline.
// Partial UTF-8 sequences are buffered; control bytes are sanitized.
//
// Prefer Capture for external process stdout/stderr: DebugWriter is filtered by
// DebugLevel (default LevelInfo drops all lines) and is the wrong dialect for
// child-command evidence used in Fail Detail. Use DebugWriter only when you
// intentionally want DEBUG-level journal lines (and set debugLevel(Debug)).
func (o *Run) debugWriter() io.WriteCloser {
	return record.NewLineWriter(func(line string) { o.debug(line) })
}

// WriteJSON serializes result as the stable v2 "evo.run" wire document plus
// one trailing newline (schema/run.v3.json) — the HTTP/embedding counterpart of
// FormatJSON's automatic Stdout write. It never serializes internal
// snapshots directly, and applies the same redaction Result's Conclusion
// already carries. HTTP status (or any other transport-level outcome) is
// the embedding application's own concern; Evo's outcome/exit semantics
// stay in the body (Conclusion.State/ExitCode).
func writeJSON(w io.Writer, result Result) error {
	body, err := wire.EncodeRun(asRecordResult(result), wire.Release)
	if err != nil {
		return fmt.Errorf("evo: encode evo.run document: %w", err)
	}
	body = append(body, '\n')
	_, err = w.Write(body)
	return err
}

// copyEvents is a copy of the durable events (v0.1 journal), current with the
// record: every settle the graph made is journaled before it is read.
func (o *Run) copyEvents() []recEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
	return o.rec.Events(o.cfg.maxEvents)
}

// PlainOptions configures pure plain projection (§25.4).
type plainOptions struct {
	width   int
	noColor bool
	// Verbose additionally emits per-reason name lists under a task's skip/keep
	// taxonomy line. Counts and the reason partition always render; Verbose
	// only adds the bounded (TruncateNames) name detail.
	verbose bool
	// Glyphs selects the state-glyph vocabulary. Plain projection has no live
	// TTY to detect, so GlyphsAuto (the default) resolves to GlyphsUnicode —
	// callers rendering off a known non-UTF-8 destination pass GlyphsASCII.
	glyphs glyphProfile
}

// RenderPlain projects a snapshot to plain text without terminal ownership.
func renderPlain(s snapshot, opts plainOptions) ([]byte, error) {
	glyphs := opts.glyphs
	if glyphs == glyphsAuto {
		glyphs = glyphsUnicode
	}
	return []byte(renderplain.Plain(s, opts.width, opts.noColor, opts.verbose, glyphs)), nil
}

// Snapshot returns an immutable copy of current state.
func (o *Run) snapshot() snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshotLocked()
}

// Print formats like fmt.Sprint and enqueues human-facing text (line-buffered).
// Errors are recorded on the Output and returned by Finish/Main — not ignored mid-stream.
func (o *Run) print(args ...any) {
	o.at(visibilityNormal).print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text (line-buffered).
func (o *Run) printf(format string, args ...any) {
	o.at(visibilityNormal).printf(format, args...)
}

// Print implements Printer.
func (p *printer) print(args ...any) {
	p.enqueue(fmt.Sprint(args...))
}

// Printf implements Printer.
func (p *printer) printf(format string, args ...any) {
	p.enqueue(fmt.Sprintf(format, args...))
}

// MarkWriterAsCharDevice treats w as a TTY during construction so tests can
// exercise live-region / color inference without opening a pty. Only this
// writer matches; other tests' writers still use the OS check.
func markWriterAsCharDevice(w io.Writer) func() { return terminal.MarkCharDevice(w) }

// IsCharDevice reports whether w is an *os.File backed by a character device
// (typical interactive TTY). Pipes, files, and non-file writers return false.
//
// Prefer Config{Stdout, Stderr} for ordinary dual-stream construction. This
// helper remains for hosts that choose Plain/NoColor from a concrete writer.
func isCharDevice(w io.Writer) bool { return terminal.IsCharDevice(w) }

// Snapshot returns the task snapshot.
func (t *Task) snapshot() taskSnapshot {
	if t == nil || t.out == nil {
		return taskSnapshot{State: pending, Progress: progress{Kind: indeterminate}}
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	if t.out.show.Rows.Task(record.TaskID(t.id)) == nil {
		return taskSnapshot{ID: t.id, State: pending, Progress: progress{Kind: indeterminate}}
	}
	return t.out.show.Rows.Snapshot(record.TaskID(t.id))
}

// Snapshot returns the collection snapshot with derived state.
func (g *Group) snapshot() tasksSnapshot {
	if g == nil || g.out == nil {
		return tasksSnapshot{State: empty}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	if g.out.show.Rows.Group(g.id) == nil {
		return tasksSnapshot{ID: g.id, State: empty}
	}
	return g.out.show.Rows.Collection(g.id)
}

// DefaultVisibleNames is how many names TruncateNames keeps before summarizing.
const defaultVisibleNames = project.DefaultVisibleNames

// TruncateNames joins names for a skip/kept-style summary.
func truncateNames(names []string, visible int) string {
	if visible <= 0 {
		visible = defaultVisibleNames
	}
	return project.TruncateNames(names, visible)
}

// Snapshot returns the sequence snapshot with derived state.
func (g *Sequence) snapshot() tasksSnapshot {
	if g == nil || g.tasks == nil {
		return tasksSnapshot{State: empty}
	}
	return g.tasks.snapshot()
}

// Context reports the cancellation signal this task's work runs under — the
// run's own (see Output.Context), so a Define or Effect callback
// doing I/O selects on it and stops when the run is interrupted.
func (t *Task) context() context.Context {
	if t == nil {
		return context.Background()
	}
	return t.out.context()
}

// Basis declares freshness inputs for this Task. Calls accumulate. Basis is
// Task configuration frozen at Define: a call after Define is ignored.
func (t *Task) basis(inputs ...fingerprint) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	if !st.neverDefined() {
		return t
	}
	sources := make([]basisSource, 0, len(inputs))
	for _, in := range inputs {
		if in != nil {
			sources = append(sources, fingerprintBasis(in))
		}
	}
	o.freshness.Tasks().AddInputs(st.node.TaskID(), sources...)
	return t
}

// nextSelf is a ProblemOption that attaches a command action re-running the
// caller's own binary with args — a self-referencing remedy ("rerun with
// --apply") that doesn't restate which binary to run (I6). Uses the same
// identity source as Confirm's PolicyFlag / I2's Fail fallback: Config.Title
// when set, else the binary's own basename. Use NextCommand instead when the
// remedy is a different (foreign) tool.
func (o *Run) nextSelf(args ...string) ProblemOption {
	return nextCommand(o.selfName(), args...)
}

// Bytes sets absolute byte progress (units and rate formatting).
func (t *Task) bytes(completed, total int64) *Task {
	return t.setProgress(completed, total, bytesKind)
}

// Fail resolves the task as failed. This is a statement, not a fluent
// chain — Fail returns nothing, so a bare `task.Fail("summary")` is
// errcheck-clean. A nil *Task is safe and resolves nothing. Inside
// Define, return the error as well so Wait sees the callback's own result.
func (t *Task) fail(summary string, options ...ProblemOption) {
	t.resolveWithProblem(failed, summary, options)
}

// neverDefined reports whether st is still waiting on its caller: declared,
// never Defined, and not resolved by a verb either.
func (st *taskState) neverDefined() bool { return st.node.NeverDefined() }

// followRecordAfterWait is what a Wait owes its caller once the graph let it
// go: the render state has answered every settle the Wait saw. A Task's done
// channel closes inside the graph's critical section, before the resolution
// that settled it has painted and filed the row. Taking o.mu here waits out
// that resolution (it holds o.mu from settle to commit) and follows the
// record for a settle made outside one, so a caller that reads the frame
// right after Wait sees the settled row.
func (o *Run) followRecordAfterWait() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
}

// anyBlockedSoFar reports whether any Task is currently in the Blocked
// state — a live, mid-run check (C12: named "SoFar" to distinguish it from
// Conclusion.AnyBlocked, which reports the finished run's final verdict;
// the two answer different questions and previously shared one name).
// Internal only (P6 deletion census: no consumer needed the mid-run
// question once Run/Conclusion existed) — see anyblocked_package_test.go.
func (o *Run) anyBlockedSoFar() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, t := range o.tasks {
		if t.node.Rec.State() == refused {
			return true
		}
	}
	return false
}

func (o *Run) nextSelfForTest(args ...string) ProblemOption { return o.nextSelf(args...) }

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FingerprintBasis.
type basisSource = freshness.BasisSource

// FingerprintBasis observes a Fingerprint (FSPath, Value, App).
func fingerprintBasis(f fingerprint) basisSource { return freshness.FingerprintBasis(f) }

type problemSeverity = record.ProblemSeverity

// Severity sets a Problem's severity.
func severity(value problemSeverity) ProblemOption {
	return problemOption{record.WithSeverity(value)}
}

// Count sets a quantity and optional unit.
func count(value int64, unit ...string) ProblemOption {
	return problemOption{record.WithCount(value, unit...)}
}

// Location sets a source location on a Problem.
func location(path string, line, column int) ProblemOption {
	return problemOption{record.AtLocation(path, line, column)}
}

// NextCommand attaches a recommended command action.
func nextCommand(executable string, args ...string) ProblemOption {
	return problemOption{record.WithCommand(executable, args...)}
}

// fail records an output-level failure.
func (o *Run) fail(summary string, options ...ProblemOption) {
	o.endWith(failed, applyProblemOptions(project.Text(summary), options))
}

// settleTimeout bounds how long a test waits for submitted work to make
// progress: each Task that settles restarts it, so a large run under the
// race detector is never cut off while it is still settling.
const settleTimeout = 5 * time.Second

// settlePoll is how often a test re-reads a row while it waits.
const settlePoll = time.Millisecond

// errNotSettled is what wait returns when the work did not settle in time:
// nothing the run would do on its own was going to settle it.
var errNotSettled = errors.New("evo test: work did not settle")

// errNotSucceeded is what wait returns for work that settled without
// succeeding.
var errNotSucceeded = errors.New("evo test: work did not succeed")

// wait blocks until t's row settles and the record follows it. The run never
// waits on one Task; a test observes the settle by reading the row.
func (t *Task) wait() error {
	if t == nil || t.out == nil {
		return nil
	}
	if t.rejected != nil {
		return t.rejected
	}
	node := t.out.graph.Task(t.id)
	if node == nil {
		return nil
	}
	return t.out.awaitSettled([]*graph.Task{node})
}

// wait blocks until every Task declared under g, and every builder that
// declares them, has settled.
func (g *Group) wait() error {
	if g == nil || g.out == nil {
		return nil
	}
	if g.rejected != nil {
		return g.rejected
	}
	node := g.out.graph.Container(g.id)
	if node == nil {
		return nil
	}
	return g.out.awaitSettled(containedTasks(node))
}

// wait is Sequence's counterpart to waiting on a Group.
func (s *Sequence) wait() error {
	if s == nil || s.tasks == nil {
		return nil
	}
	return s.tasks.wait()
}

// containedTasks is c's builder gate and every Task declared under c,
// directly or through nested containers.
func containedTasks(c *graph.Container) []*graph.Task {
	var tasks []*graph.Task
	if gate := c.BuilderGate(); gate != nil {
		tasks = append(tasks, gate)
	}
	tasks = append(tasks, c.Tasks()...)
	for _, child := range c.Children() {
		tasks = append(tasks, containedTasks(child)...)
	}
	return tasks
}

// awaitSettled polls until every Task in tasks is terminal, then follows the
// record so the frame shows each settled row, and joins why any did not
// succeed.
func (o *Run) awaitSettled(tasks []*graph.Task) error {
	deadline := time.Now().Add(settleTimeout)
	var failures []error
	for _, t := range tasks {
		for !record.IsTerminalTask(t.Rec.State()) {
			if time.Now().After(deadline) {
				return fmt.Errorf("%w: %s", errNotSettled, t.Name)
			}
			time.Sleep(settlePoll)
		}
		deadline = time.Now().Add(settleTimeout)
		if state := t.Rec.State(); !state.Succeeded() && state != record.Excluded {
			failures = append(failures, fmt.Errorf("%w: %s %s", errNotSucceeded, t.Name, state))
		}
	}
	o.followRecordAfterWait()
	return errors.Join(failures...)
}

// delay is a non-nil *time.Duration for a tuning field where zero is meaningful.
func delay(d time.Duration) *time.Duration { return &d }

// readFrom is the Facades that send what a test reads for output to w: the document or event stream on stdout
// for JSON and JSONL, rows on stderr otherwise. The other stream is discarded.
func readFrom(output Output, w io.Writer) Facades {
	if output == JSON || output == JSONL {
		return Facades{Stdout: w, Stderr: io.Discard}
	}
	return Facades{Stdout: io.Discard, Stderr: w}
}

// Test-only entry points into production behaviour; no production caller needs them.

func (o *Run) cancel(reason string) {
	o.graph.EndRun(o.endingName(), record.TaskInit{State: cancelled, Summary: project.Text(reason)})
}

func (t *Task) cancel(reason string) {
	t.finish(cancelled, project.Text(reason), nil)
}

// context reports the run's cancellation signal (cancelled on interrupt and close). A nil Run reports a never-cancelled context.
func (o *Run) context() context.Context {
	if o == nil {
		return context.Background()
	}
	return o.graph.Context()
}

func (o *Run) debug(message string, fields ...field) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitDebugLocked(message, fields, false)
}

func (o *Run) emitDebugLocked(message string, fields []field, force bool) {
	o.emitDebugRecordLocked("DEBUG", message, fields, time.Time{}, force)
}

func (o *Run) writeDiagnosticText(text string) {
	if o == nil || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writeDiagnosticTextLocked(text)
}

func (o *Run) writeDiagnosticTextLocked(text string) {
	if text != "" {
		humanSinks{o}.Diagnostic(text)
	}
}

func detailTail(c *record.Capture) ProblemOption {
	return problemOption{record.WithEvidenceTail(c.DetailText)}
}

type (
	progress      = record.Progress
	evidencePhase = record.EvidencePhase
)

var (
	errLimitExceeded        = misuse.ErrLimitExceeded
	errDeclaredInCallback   = misuse.ErrDeclaredInCallback
	errDuplicateSiblingName = misuse.ErrDuplicateSiblingName
)
