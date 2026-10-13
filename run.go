package evo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/bits"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/misuse"
	"github.com/zachbornheimer/evident-output/internal/process"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// errRunExitedWithoutReturn is the application error recorded when the run
// callback's goroutine ended without returning.
var errRunExitedWithoutReturn = errors.New("run callback exited without returning (runtime.Goexit)")

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

// Run executes a CLI presentation lifecycle against the package-level default
// instance (see Init) and returns the Result, never exiting — the
// package-level counterpart of Output.Run, for callers (tests, or a caller
// composing its own exit path) that need the Result without Main's exit-code
// derivation.
//
// run reports only an error; the Conclusion (0/1/2/130) is the sole source of
// the exit code — see Output.Run for the full lifecycle and signal contract.
func execute_1(ctx context.Context, run RunFunc) Result {
	return execute(ctx, run)
}

func execute(ctx context.Context, run RunFunc) Result {
	out := defaultRun()
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
//	    (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: "tool"}, func(ctx context.Context, r *evo.Run) error { return nil })
//	    os.Exit(evo.Main(run))
//	}
//
// Callers that need the full Result (Conclusion plus application error, for
// embedding or JSON projection) call Run instead.
func Execute(ctx context.Context, cfg Config, run RunFunc) Result {
	out := initRun(cfg)
	if ctx == nil {
		ctx = context.Background()
	}
	return runInterruptible(ctx, out, run)
}

func Main(run RunFunc, cfg ...Config) int {
	c := Config{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	out := initRun(c)
	return runInterruptible(context.Background(), out, run).ExitCode()
}

// runInterruptible executes run to completion, wiring SIGINT/SIGTERM into
// cancellation of the active task (or the output itself) — and of the
// derived ctx passed to run — so the ledger and exit code always agree. A
// second signal returns ExitCancelled immediately instead of waiting for run
// to unwind — the process-level os.Exit that wraps a caller's
// os.Exit(evo.Main(run)) is what actually terminates.
func runInterruptible(ctx context.Context, out *Run, run RunFunc) Result {
	sigCh := make(chan process.Signal, signalChannelCapacity)
	notifySignals(sigCh, process.Interrupt, process.Terminate)
	defer stopSignals(sigCh)

	// runCtx becomes o.Context() for the duration of this run (see
	// beginRunContext): every Define/Verify task scope started from here
	// on descends from the caller's own ctx, not just from run's local
	// parameter.
	//
	// Cancellation is detached from the caller's ctx on purpose: the watcher
	// below records the Cancel first (out.interrupt), then cancels the run,
	// so a task that sees ctx.Done ends Cancelled rather than Failed with the
	// caller's deadline error.
	runCtx := out.beginRunContext(watchedContext{ctx})

	// The whole lifecycle (body, then the Finish drain) runs in one
	// goroutine so the signal watcher below stays armed until the Result
	// exists.
	var interrupted atomic.Bool
	done := make(chan Result, 1)
	go func() {
		// Delivered from a defer so a callback that ends its goroutine via
		// runtime.Goexit (directly, or through a library such as
		// testing.T.FailNow) still releases the waiter below.
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
			if out.cleanStops.stop(sig) {
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

// awaitInterrupted waits for the interrupted run's Result; a second signal
// returns Cancelled at once instead of waiting for the run to unwind.
func awaitInterrupted(sigCh <-chan process.Signal, done <-chan Result) Result {
	select {
	case result := <-done:
		return result
	case <-sigCh:
		return Result{Conclusion: Conclusion{Outcome: Cancelled, cancelled: true, exitCode: exitCancelled}}
	}
}

// watchedContext keeps the caller's deadline and values but not its
// cancellation: the run's watcher is the only canceller, so it records the
// Cancel before any task observes Done.
type watchedContext struct{ parent context.Context }

func (w watchedContext) Deadline() (time.Time, bool) { return w.parent.Deadline() }
func (w watchedContext) Done() <-chan struct{}       { return nil }
func (w watchedContext) Err() error                  { return nil }
func (w watchedContext) Value(key any) any           { return w.parent.Value(key) }

// concludeInterruptible finalizes the run once its body returned: a run
// that was signalled concludes Cancelled, any other as an ordinary run.
func concludeInterruptible(out *Run, interrupted *atomic.Bool, runErr error) Result {
	if interrupted.Load() {
		return concludeCancelled(out, runErr)
	}
	return concludeRun(out, runErr)
}

// concludeRunAfterExit finalizes a run whose body ended its goroutine
// without returning.
func concludeRunAfterExit(out *Run, interrupted *atomic.Bool, returned bool, runErr error) Result {
	if !returned {
		runErr = errRunExitedWithoutReturn
	}
	return concludeInterruptible(out, interrupted, runErr)
}

// concludeRun reconciles an ordinary (non-interrupted) run outcome into the
// Result.
func concludeRun(out *Run, runErr error) Result {
	if runErr != nil && !out.anyFailed() {
		// Synchronize the presentation model with the application error only when
		// no entity already recorded Failed — avoids a duplicate synthetic Fail row
		// on top of an existing task/item Fail. Exit code still comes from conclusion.
		out.fail(runErr.Error())
	}
	finishErr := out.finish()
	closeErr := out.close()
	conclusion := out.conclusion()
	// Bookkeeping misuse (a leftover unresolved task, a duplicate key, ...)
	// is already folded into the Conclusion itself before the band renders
	// (release-gate finding 2) — ExitCode already agrees with what printed.
	// A genuine renderer/write failure is different: it surfaces only after
	// the band is already flushed, so the band never had a chance to
	// reflect it — that still escalates an otherwise-OK exit code here.
	if conclusion.exitCode == exitOK && (errors.Is(finishErr, errRenderer) || errors.Is(closeErr, errRenderer)) {
		conclusion.exitCode = exitFailed
	}
	return Result{Conclusion: conclusion, Err: runErr}
}

// concludeCancelled finalizes an interrupted run; the Conclusion computed
// from the Cancel already recorded is the sole source of ExitCancelled.
// runErr is the (usually context.Canceled-flavored) error run returned after
// cancellation, carried through as Result.Err for embedders.
func concludeCancelled(out *Run, runErr error) Result {
	_ = out.finish()
	_ = out.close()
	return Result{Conclusion: out.conclusion(), Err: runErr}
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

// anyFailed reports whether any Task is currently Failed.
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

// --- folded from construct.go ---

// processArgv0 is the facade over the program name (facade rule: no direct
// os.Args read anywhere else) — a var so tests can inject a fixed value
// instead of depending on the real test binary's path.
var processArgv0 = func() string {
	args := process.Args()
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// identityFallbackName is the executable's own basename, used only when an
// output-level outcome (Output.Fail/Cancel) has no named task and no
// explicit Config.Title to identify it with — replacing the generic literal
// "command" with the caller's actual binary name (I2). This is deliberately
// NOT plumbed into Snapshot.Subject / the conclusion band's Subject: Config.
// Title stays the only way to set that, and many existing goldens depend on
// the "no Subject configured" fallback (bare state name) staying exactly as
// it is. Separately, when a DryRun run's Config.Subject header rendered and
// the derived verdict settled on a pure StatePlanned (no failed/blocked/
// warned/partial/cancelled), the trailing conclusion band itself is
// suppressed as redundant with that header — see
// txt.ShouldSuppressStandaloneConclusion and evo-rec.md's dry-run
// section.
func identityFallbackName() string {
	if base := filepath.Base(processArgv0()); base != "." && base != string(filepath.Separator) {
		return base
	}
	return "command"
}

// ColorMode selects color policy. The zero value is automatic.
type colorMode int

const (
	// ColorAuto uses TTY detection and honors NO_COLOR.
	colorAuto colorMode = iota
	// ColorAlways forces semantic color even off a TTY.
	colorAlways
	// ColorNever disables color.
	colorNever
)

// Format selects the overall projection mode. Zero is ordinary human output.
type format int

const (
	// FormatHuman is ordinary human (and optional interactive) presentation.
	formatHuman format = iota
	// FormatData reserves stdout for the application's domain payload; Evo
	// human presentation goes to stderr (data-command mode).
	formatData
	// FormatExternal disables inline rendering (snapshots only).
	formatExternal
	// FormatJSON writes one final v2 "evo.run" document to Stdout at Finish;
	// human presentation still goes to Stderr, exactly as FormatData routes
	// it (spec §32.1). Do not infer this from stdout being a pipe — it is
	// always an explicit top-level choice.
	formatJSON
	// FormatJSONL streams v2 "evo.event" JSON lines to Stdout as they occur,
	// plus a final run.finished line; human presentation goes to Stderr
	// (spec §32.1).
	formatJSONL
)

// ParseFormat parses one of "human", "data", "external", "json", "jsonl"
// (case-insensitive, surrounding whitespace ignored) into a Format — the
// public entry point a host CLI's own --format/--json flag binds to (spec
// §32.1). Evo does not parse os.Args itself.
func parseFormat(s string) (format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "human":
		return formatHuman, nil
	case "data":
		return formatData, nil
	case "external":
		return formatExternal, nil
	case "json":
		return formatJSON, nil
	case "jsonl":
		return formatJSONL, nil
	default:
		return 0, fmt.Errorf("%w: unknown format %q", errInvalidConfig, s)
	}
}

// Verbosity selects which message visibilities are projected to the human stream.
// Zero is normal (non-verbose) human detail.
type verbosity int

const (
	// VerbosityNormal projects Normal-visibility messages only.
	verbosityNormal verbosity = iota
	// VerbosityVerbose also projects Verbose-visibility messages. It also
	// expands each Task.Skipped/Kept taxonomy row from its default
	// aggregated "! skipped N (reason1, reason2)" count into one named line
	// per reason ("reason: name1, name2, ..."). The names themselves are
	// never lost at VerbosityNormal — they are always present on the Go
	// TaskSnapshot.Skipped/Kept fields (returned by Output.Snapshot and
	// Task.Snapshot); VerbosityVerbose only changes whether the plain
	// human render surfaces them. The wire JSONDocument (JSONTask) does not
	// currently carry Skipped/Kept at all — read the Go snapshot directly to
	// get the names programmatically.
	verbosityVerbose
)

// DebugConfig configures the debug journal presentation.
type debugConfig struct {
	// Level is the minimum debug journal level. Zero (LevelUnset) resolves to
	// LevelInfo. Use LevelTrace or LevelDebug to surface Debug/Capture mirrors.
	level logLevel
	// View selects history vs pane presentation (default History).
	view debugPresentation
	// PaneHeight is used when View is DebugPresentationPane (default 5).
	paneHeight int
	// NewestFirst orders the pane (default true when zero-config pane).
	newestFirst *bool
	// PreserveAlways forces a diagnostic tail on every Finish in pane mode.
	preserveAlways bool
	// AddSource resolves each record's call site to a source=file.go:line
	// field on human/pane/history rendering (slog.HandlerOptions.AddSource
	// semantics). Off by default: the raw program counter is always kept on
	// LogRecord.PC for machine consumers, but a human debug line never shows
	// a bare pc=<uintptr> unless this is set.
	addSource bool
}

// Config is the sole application-facing construction surface.
//
// Zero values mean automatic/default behavior. Use DefaultConfig() when you
// need a mutable baseline for advanced fields.
//
//	out := (*evo.Run)(nil); _ = evo.Execute(context.Background(), , func(ctx context.Context, r *evo.Run) error { return nil })
//	out := (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: "bpp-csharp"}, func(ctx context.Context, r *evo.Run) error { return nil })
//	cfg := evo.Config{}; cfg.Name = "x"; out := (*evo.Run)(nil); _ = evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error { return nil })
type Config struct {
	Name string
	// Title is the 1.x subject shown in the conclusion.
	title   string
	Verbose bool
	Output  Output
	State   State
	Facades Facades

	// Subject is an optional durable line rendered once, immediately, right
	// under the title — a repo path, a target host, the thing every
	// projection needs the reader to see up front. Set it once in Config
	// instead of calling fmt.Fprintln(r.Text(), root) (or whatever the identifying
	// value is) at every projection/command that needs to show it.
	//
	// When the subject text isn't known until after Init (e.g. resolved
	// from a flag parsed later, but still before any other I/O), call
	// Output.subject(text) instead — same one-shot durable-line semantics,
	// as a post-construction setter (I3).
	subject string

	// Facts are run-scoped name/value lines painted with the header, before
	// any Task. Use them for values known at construction (scan roots, the
	// repo path). Discovery that happens during work belongs on the Task
	// that learned it (task.Fact), not here.
	facts []factRecord

	// Stdout is the ordinary human stream (default os.Stdout).
	// In FormatData mode, Stdout is reserved for domain payload via ResultWriter;
	// human presentation moves to Stderr.
	stdout io.Writer
	// Stderr owns diagnostics by default (default os.Stderr).
	stderr io.Writer
	// Result is an optional domain-payload writer. When nil and Format is
	// FormatData, ResultWriter returns Stdout. Presentation never writes here.
	result io.Writer
	// Stdin is the facade Confirm reads one answer line from (default os.Stdin).
	stdin io.Reader

	// Verbosity gates Verbose() print messages (default VerbosityNormal).
	verbosity verbosity
	// Color policy (default ColorAuto).
	color colorMode
	// Format projection mode (default FormatHuman). Stream routing only —
	// FormatData does not mean JSON. Presentation encoding is Projection.
	format format
	// Projection selects presentation encoding (human, plain, json, jsonl). Zero is unspecified: Init applies EVO_OUTPUT when set,
	// otherwise human TTY/plain inference. Independent of Format.
	projection projection

	// Debug configures the debug journal.
	Debug debugConfig

	// Advanced (optional) — zero values inherit safe defaults.
	clock    timeSource
	redactor Redactor
	terminal terminalDriver
	strict   bool
	width    int
	// VisibilityDelay is the wait before the first live paint.
	// nil means default (80ms). Non-nil is exact, including 0 for immediate.
	// Use evo.Delay(d) to set a value from a duration literal.
	visibilityDelay *time.Duration
	maxFrameRate    int
	maxEntities     int
	maxEvents       int
	// Plain disables live interactive frames, on a TTY or off (C3: replaces
	// the former separate ForcePlain and NonInteractive fields — every read
	// site combined them with OR, so there was never a distinct behavior
	// between the two to preserve).
	plain bool

	// Glyphs selects the state-glyph vocabulary (default GlyphsAuto: Unicode
	// off a TTY or on a UTF-8 locale, ASCII on a non-UTF-8 interactive TTY).
	glyphs glyphProfile

	// FailedExitCode is the process exit code when the conclusion is failed.
	// Zero means use ExitFailed (2). Set to 1 for conventional CLI tools that
	// treat any non-zero failure as exit 1 (e.g. quality gates / git hooks).
	failedExitCode int

	// DryRun declares this run a dry run once, for the whole process: every
	// evo.Effect, evo.File, and evo.Exec call renders as
	// a [planned] row with the imperative verb
	// instead of a [changed] row with the past-tense verb. No call site writes
	// its own tense.
	DryRun bool

	// Preview declares this run a preview before a confirm gate: the same
	// planned tense as DryRun — Effect callbacks never run and every
	// Effect/File call renders as a [planned] row with the
	// imperative verb — announced with the caller's own Config.Subject
	// ("repo <path>") instead of the "[dry-run] <subject>" header, and with
	// the same redundant-band suppression on a pure planned verdict.
	//
	// A preview is about to ask permission. Labelling it a dry run tells the
	// user nothing will happen and then asks them to authorize it; the
	// dialect's screenshot regression names that exact contradiction. Use
	// DryRun for `--dry-run`, which really does stop; Preview for the plan a
	// confirm gate is about to act on.
	preview bool

	// Isolated returns an independent Output that is not installed as the
	// package-level default. First paint still arms — Isolated is not a
	// blank-terminal exemption. Use for parallel tests and embedders that
	// hold their own *Run instead of going through Default()/Task().
	// This is the one and only opt-out from default installation.
	isolated bool

	// MaxConcurrency is the scheduler ceiling: it bounds every executing
	// callback, including work a waiting goroutine runs itself. Zero means
	// GOMAXPROCS.
	MaxConcurrency int

	// StateDir, when non-empty, is the exact manifest state directory File's
	// reconciliation history is read from and committed to
	// (<StateDir>/manifest-v1.json) — bypassing the default
	// os.UserCacheDir()/evident-output/<app-id>/<workspace-hash>/ derivation
	// entirely (spec §11.3). Tests set this to t.TempDir() for isolation.
	stateDir string

	// AppID overrides the derived application id (main-module path plus
	// executable basename) used in the default manifest cache path.
	// Ignored when StateDir is set.
	appID string

	// ProcessRunner overrides the facade evo.Exec spawns child processes
	// through (spec §8.4). Nil uses the real OS process runner; tests
	// inject testkit's scripted fake for deterministic Exec coverage.
	processRunner processRunner

	// FileFS overrides the facade evo.File performs filesystem I/O through
	// (spec §8.2). Nil uses the real OS filesystem; tests inject a fake to
	// script a reconciliation outcome (e.g. a chmod failure) deterministically.
	fileFS fileFS
}

// Delay returns a non-nil *time.Duration for Config fields where zero is meaningful.
//
//	cfg.Name /*VisibilityDelay*/ = evo.Delay(0)                      // immediate
//	cfg.Name /*VisibilityDelay*/ = evo.Delay(80 * time.Millisecond) // explicit default
func delay(d time.Duration) *time.Duration {
	return &d
}

// DefaultConfig returns a fresh ordinary CLI configuration.
// Mutating the result does not affect later DefaultConfig() calls.
func defaultConfig() Config {
	return Config{
		// Stdout/Stderr filled at resolve time so tests can still pass buffers
		// without forcing os.Stdout into DefaultConfig equality checks.
		Debug: debugConfig{
			level: levelInfo,
			view:  debugPresentationHistory,
		},
		width:           defaultWidth,
		visibilityDelay: delay(defaultVisibilityDelay),
		maxFrameRate:    defaultMaxFrameRate,
		maxEntities:     defaultMaxEntities,
		maxEvents:       defaultMaxEvents,
	}
}

// resolveConfig fills zero-value fields with ordinary CLI defaults.
func resolveConfig(c Config) Config {
	base := defaultConfig()
	if c.stdout == nil {
		if c.Facades.Stdout != nil {
			c.stdout = c.Facades.Stdout
		} else {
			c.stdout = terminal.Stdout()
		}
	}
	if c.stderr == nil {
		if c.Facades.Stderr != nil {
			c.stderr = c.Facades.Stderr
		} else {
			c.stderr = terminal.Stderr()
		}
	}
	if c.stdin == nil {
		if c.Facades.Stdin != nil {
			c.stdin = c.Facades.Stdin
		} else {
			c.stdin = terminal.Stdin()
		}
	}
	if c.clock == nil && c.Facades.Clock != nil {
		c.clock = c.Facades.Clock
	}
	if c.terminal == nil && c.Facades.Terminal != nil {
		c.terminal = c.Facades.Terminal
	}
	if c.redactor == nil && c.Facades.Redactor != nil {
		c.redactor = c.Facades.Redactor
	}
	if c.processRunner == nil && c.Facades.Process != nil {
		c.processRunner = c.Facades.Process
	}
	if c.fileFS == nil && c.Facades.FS != nil {
		c.fileFS = c.Facades.FS
	}
	if c.stateDir == "" && c.State.Dir != "" {
		c.stateDir = c.State.Dir
	}
	if c.appID == "" && c.State.AppID != "" {
		c.appID = c.State.AppID
	}
	if c.title == "" {
		c.title = c.Name
	}
	if c.Verbose {
		c.verbosity = verbosityVerbose
	}
	switch c.Output {
	case Plain:
		c.plain = true
		c.projection = projectionPlain
	case JSON:
		c.format = formatJSON
		c.plain = true
	case JSONL:
		c.format = formatJSONL
		c.plain = true
	}
	// LevelUnset (zero) → LevelInfo. LevelTrace is non-zero and selectable via Config.
	if c.Debug.level == levelUnset {
		c.Debug.level = levelInfo
	}
	if c.width <= 0 {
		c.width = base.width
	}
	// nil = unspecified → default; non-nil (including 0) is intentional.
	if c.visibilityDelay == nil {
		c.visibilityDelay = delay(defaultVisibilityDelay)
	}
	if c.maxFrameRate <= 0 {
		c.maxFrameRate = base.maxFrameRate
	}
	if c.maxEntities <= 0 {
		c.maxEntities = base.maxEntities
	}
	if c.maxEvents <= 0 {
		c.maxEvents = base.maxEvents
	}
	if c.clock == nil {
		c.clock = systemClock{}
	}
	if c.redactor == nil {
		c.redactor = process.NoopRedactor{}
	}
	if c.processRunner == nil {
		c.processRunner = process.System()
	}
	if c.fileFS == nil {
		c.fileFS = osFileFS{}
	}
	return c
}

func newFromConfig(c Config) *Run {
	opts := configToOptions(c)
	return newOutput(c.title, opts...)
}

// withVerbosity stores verbosity on the internal config.
func withVerbosity(v verbosity) option {
	return optionFunc(func(c *config) { c.verbosity = v })
}

// withFailedExitCode stores a non-default failed conclusion exit code.
func withFailedExitCode(code int) option {
	return optionFunc(func(c *config) { c.failedExitCode = code })
}

// --- folded from construct_options.go ---

// configToOptions translates Config into the internal Option list, one
// concern at a time. Options apply in order, so a later concern may refine
// an earlier one (plain() after a live terminal, for example).
func configToOptions(c Config) []option {
	var opts []option
	opts = append(opts, streamOptions(c)...)
	opts = append(opts, colorOptions(c)...)
	opts = append(opts, terminalOptions(c)...)
	opts = append(opts, runtimeOptions(c)...)
	opts = append(opts, debugOptions(c.Debug)...)
	opts = append(opts, modeOptions(c)...)
	return opts
}

// humanWriter is where a Format routes human presentation: Stderr when
// machine output owns Stdout (FormatData and the wire formats, spec
// §32.1), Stdout otherwise.
func humanWriter(c Config) io.Writer {
	switch c.format {
	case formatData, formatJSON, formatJSONL:
		return c.stderr
	default:
		return c.stdout
	}
}

// streamOptions routes each stream by Format.
func streamOptions(c Config) []option {
	switch c.format {
	case formatData:
		// Human presentation on stderr; domain payload on Result (default Stdout).
		resultW := c.result
		if resultW == nil {
			resultW = c.stdout
		}
		return []option{to(c.stderr), withDiagnostics(c.stderr), dataProjection(), resultStream(resultW)}
	case formatExternal:
		opts := []option{to(c.stdout), withDiagnostics(c.stderr), externalProjection()}
		if c.result != nil {
			opts = append(opts, resultStream(c.result))
		}
		return opts
	case formatJSON, formatJSONL:
		// Human presentation on stderr, exactly like FormatData; the v2
		// wire document/event stream is Evo's own write (not the app's
		// domain payload), so it uses wireStream/wireFormat rather than
		// Result/resultStream (spec §32.1: "stdout: exactly one final JSON
		// document"/"stdout: JSON event lines only" — never c.Result, which
		// stays the FormatData-only escape hatch).
		return []option{to(c.stderr), withDiagnostics(c.stderr), wireFormatOption(c.format), wireStreamOption(c.stdout)}
	default:
		opts := []option{to(c.stdout), withDiagnostics(c.stderr)}
		if c.result != nil {
			opts = append(opts, resultStream(c.result))
		}
		return opts
	}
}

// colorOptions turns color off for ColorNever, and for ColorAuto when
// NO_COLOR is set or the human writer is not a terminal.
func colorOptions(c Config) []option {
	noColor := false
	switch c.color {
	case colorNever:
		noColor = true
	case colorAlways:
	default: // ColorAuto
		noColor = lookupEnv(envKeyNoColor) != "" || !writerIsCharDevice(humanWriter(c))
	}
	if noColor {
		return []option{withNoColor()}
	}
	return nil
}

// terminalOptions picks the presentation surface: a caller-supplied
// driver, an interactive live region on a real TTY, or plain output.
//
// FormatData and the v2 wire formats all route human presentation to
// Stderr alike (spec §32.1), so the live region belongs on Stderr for all
// three, or a JSON/JSONL CLI run from an interactive terminal would
// silently regress to the durable plain renderer merely because machine
// output owns Stdout.
func terminalOptions(c Config) []option {
	var opts []option
	liveWriter := humanWriter(c)
	wantLive := !c.plain && rendersInline(c.format) && writerIsCharDevice(liveWriter)
	switch {
	case c.terminal != nil:
		opts = callerTerminalOptions(c)
	case wantLive:
		opts = liveTerminalOptions(c, liveWriter)
	default:
		opts = []option{plain()}
	}
	if c.plain || c.projection.forcesPlain() {
		opts = append(opts, plain())
	}
	return append(opts, withProjection(c.projection))
}

// rendersInline reports whether f presents inline at all; FormatExternal
// is snapshots only.
func rendersInline(f format) bool {
	switch f {
	case formatHuman, formatData, formatJSON, formatJSONL:
		return true
	default:
		return false
	}
}

// callerTerminalOptions wires a caller-supplied driver (Config.Terminal).
//
// The driver may still write to one of the two streams Config already
// wired up — DETECT that via the driver's own Sink() rather than requiring
// the caller to say so, closing the examples/terminal-driver double-band
// gap (X3). Both configured streams count: a driver aimed at the OTHER
// configured stream (e.g. Stderr while primary defaults to Stdout — every
// doc/example) still owns rendering there and must not also be duplicated
// onto primary (release-gate round 9 finding 1). A driver aimed at a third
// stream (e.g. a log file) takes primary with it: Config exposes no
// separate to() a caller could use to request an intentional second copy,
// and otherwise nothing would ever fan the driver's own conclusion band
// out to a plain/non-interactive mirror (finding-2 symmetry).
func callerTerminalOptions(c Config) []option {
	opts := []option{withTerminal(c.terminal)}
	sr, ok := c.terminal.(sinkReporter)
	if !ok {
		return opts
	}
	if sink := sr.Sink(); sink != nil {
		switch sink {
		case c.stdout, c.stderr:
			opts = append(opts, withPrimarySharesTerminal())
		default:
			opts = append(opts, to(sink))
		}
	}
	if sameTerminalDevice(sr.Sink(), c.stderr) {
		opts = append(opts, withDiagnosticSharesTerminal())
	}
	return opts
}

// liveTerminalHeight is the live region's height when the terminal cannot
// report its own.
const liveTerminalHeight = 24

// liveTerminalOptions wires the interactive live region onto liveWriter,
// a real TTY.
func liveTerminalOptions(c Config, liveWriter io.Writer) []option {
	width, height := c.width, liveTerminalHeight
	if width <= 0 {
		width = defaultWidth
	}
	// Prefer real terminal dimensions when liveWriter is a TTY *os.File. A
	// TTY without ioctl size (pty, ssh, `timeout`) keeps the default
	// geometry so a spinner still appears.
	if tw, th, ok := terminal.WriterSize(liveWriter); ok {
		// Caller Width>0 is a deterministic override; otherwise use real cols.
		if c.width <= 0 || c.width == defaultWidth {
			width = tw
		}
		height = th
	}
	ansiOpts := []terminal.Option{
		terminal.WithInteractive(true),
		terminal.WithSize(width, height),
		// Re-query geometry on each live redraw (resize-aware path).
		terminal.WithSizeWriter(liveWriter),
	}
	opts := []option{
		withTerminal(terminal.NewANSI(liveWriter, ansiOpts...)),
		withWidth(width),
		// liveWriter is the same stream to() was already given (c.Facades.Stdout,
		// or c.Facades.Stderr when machine output owns Stdout) — the terminal and
		// primary are one physical destination, so Finish must not
		// dual-write the conclusion band a second time.
		withPrimarySharesTerminal(),
	}
	// withDiagnostics(c.Facades.Stderr) is a distinct io.Writer from liveWriter in
	// the realistic default (to(Stdout), withDiagnostics(Stderr)), but on an
	// interactive shell without redirection both fds name the same
	// controlling tty — detect that here so Debug routes through live-aware
	// sequencing instead of a raw dual-stream write (gate-7 finding 1).
	if sameTerminalDevice(liveWriter, c.stderr) {
		opts = append(opts, withDiagnosticSharesTerminal())
	}
	return opts
}

// runtimeOptions carries the run's facades and limits.
func runtimeOptions(c Config) []option {
	var opts []option
	if c.stdin != nil {
		opts = append(opts, stdin(c.stdin))
	}
	visDelay := defaultVisibilityDelay
	if c.visibilityDelay != nil {
		visDelay = *c.visibilityDelay
	}
	return append(opts,
		withClock(c.clock), redact(c.redactor), withWidth(c.width),
		visibilityDelay(visDelay), maxFrameRate(c.maxFrameRate),
		maxEntities(c.maxEntities), maxEvents(c.maxEvents),
		maxConcurrency(c.MaxConcurrency),
		withStateDir(c.stateDir), withAppID(c.appID),
		withProcessRunner(c.processRunner),
		withFileFS(c.fileFS),
	)
}

// debugOptions configures the debug journal and its presentation.
func debugOptions(d debugConfig) []option {
	opts := []option{debugLevel(d.level)}
	if d.addSource {
		opts = append(opts, debugAddSource())
	}
	if d.view != debugPresentationPane {
		return append(opts, debugHistory())
	}
	var paneOpts []debugPaneOption
	if d.paneHeight > 0 {
		paneOpts = append(paneOpts, paneHeight(d.paneHeight))
	}
	if d.newestFirst != nil {
		if *d.newestFirst {
			paneOpts = append(paneOpts, newestFirst())
		} else {
			paneOpts = append(paneOpts, oldestFirst())
		}
	}
	if d.preserveAlways {
		paneOpts = append(paneOpts, preserveDebugTail())
	}
	return append(opts, debugPane(paneOpts...))
}

// modeOptions carries the run's modes: strictness, verbosity, exit code,
// dry run and preview, and glyphs.
func modeOptions(c Config) []option {
	var opts []option
	if c.strict {
		opts = append(opts, strict())
	}
	opts = append(opts, withVerbosity(c.verbosity))
	if c.failedExitCode != 0 {
		opts = append(opts, withFailedExitCode(c.failedExitCode))
	}
	if c.DryRun || c.preview {
		opts = append(opts, dryRun())
		if c.subject != "" {
			opts = append(opts, dryRunHeader(c.subject))
		}
	}
	if c.preview {
		opts = append(opts, preview())
	}
	return append(opts, glyphs(c.glyphs))
}

// --- folded from default.go ---

// defaultMu guards defaultOut, the package-level default instance installed
// by Init/SetDefault and read by Default and the package-level Task/Item/
// Print* facades below (mirrors slog.SetDefault).
var (
	defaultMu  sync.RWMutex
	defaultOut *Run
)

// Init is the sole Output constructor. It builds an Output from cfg,
// installs it as the package-level default, and arms first paint — call
// once, in main, before any I/O:
//
//	func main() {
//	    (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: "repo-retire"}, func(ctx context.Context, r *evo.Run) error { return nil })
//	    os.Exit(evo.Main(run))
//	}
//
// (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error { return nil }) (or (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error { return nil })) builds an
// ordinary default instance. Config.Isolated returns an independent
// instance that is not installed as the package default. First paint still
// arms — Isolated is not a blank-terminal exemption.
//
// Init fills still-zero Config fields from EVO_OUTPUT, EVO_VERBOSE,
// EVO_DEBUG, NO_COLOR, and FORCE_COLOR before TTY inference.
// Explicit Config values win over env; env wins over TTY.
func initRun(configs ...Config) *Run {
	cfg := resolveInitConfig(configs)
	cfg = applyEnv(cfg)
	resolved := resolveConfig(cfg)
	out := newFromConfig(resolved)
	return finishInit(out, cfg, resolved.facts)
}

// finishInit applies the package-default install (unless Isolated) and the
// first-paint invariant. Isolated skips SetDefault only.
func finishInit(out *Run, cfg Config, facts []factRecord) *Run {
	if !cfg.isolated {
		setDefault(out)
	}
	out.changes.Workspace() // captured at Run start: relative paths never follow a later chdir
	out.arm()
	if cfg.subject != "" && !cfg.DryRun && !cfg.preview {
		out.println(cfg.subject)
	}
	for _, fact := range facts {
		out.Fact(fact.Name, fact.Value)
	}
	return out
}

// resolveInitConfig picks Init's effective Config from its variadic
// argument: zero args is the zero Config ((*evo.Run)(nil); _ = evo.Execute(context.Background(), , func(ctx context.Context, r *evo.Run) error { return nil })), and one or more
// uses the first.
func resolveInitConfig(configs []Config) Config {
	if len(configs) == 0 {
		return Config{}
	}
	return configs[0]
}

// SetDefault installs out as the package-level default Output.
func setDefault(out *Run) {
	defaultMu.Lock()
	defaultOut = out
	defaultMu.Unlock()
}

// Default returns the package-level default Output, lazily creating one with
// a zero Config the first time it's needed — package-level Task/Print*
// never panic even when the caller skipped Init.
func defaultRun() *Run {
	defaultMu.RLock()
	out := defaultOut
	defaultMu.RUnlock()
	if out != nil {
		return out
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultOut == nil {
		defaultOut = newFromConfig(resolveConfig(applyEnv(Config{})))
	}
	return defaultOut
}

// Task declares a Task on the default instance. A repeated name is a
// duplicate sibling declaration (§3.1), not a get-or-create — see
// Output.Task.
func defaultTask(name string) *Task {
	return defaultRun().Task(name)
}

// Reason returns a get-or-create taxonomy Reason by name on the default
// instance registry — duplicate strings merge into one bucket, so an inline
// evo.Reason("protected") at every call site is always legal; lifting it to a
// package var (var reasonProtected = evo.Reason("protected")) is optional,
// not required for correctness.
func reason(name string) taxonomyReason {
	return defaultRun().reasonGetOrCreate(name)
}

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func print(args ...any) {
	defaultRun().print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func printf(format string, args ...any) {
	defaultRun().printf(format, args...)
}

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func println(args ...any) {
	defaultRun().println(args...)
}

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func verbose() *printer {
	return defaultRun().at(visibilityVerbose)
}

// SlogHandler returns a slog.Handler journaling to the default instance —
// package-level sugar (release-gate round 8 finding 6) matching Task/Verbose:
// a caller using the default-instance facade throughout a run should never
// have to reach for a hosted *Run just for the slog bridge. See
// Output.SlogHandler for the level policy and full contract.
func slogHandler() slog.Handler {
	return defaultRun().slogHandler()
}

// --- folded from env.go ---

// Environment keys honored at Init when the matching Config field is unset.
const (
	envKeyOutput     = "EVO_OUTPUT"
	envKeyVerbose    = "EVO_VERBOSE"
	envKeyDebug      = "EVO_DEBUG"
	envKeyNoColor    = "NO_COLOR"
	envKeyForceColor = "FORCE_COLOR"
)

const (
	envOutputHuman = "human"
	envOutputPlain = "plain"
	envOutputJSON  = "json"
	envOutputJSONL = "jsonl"

	envForceColorOff = "0"

	envVerboseOn = "1"

	envDebugInfo  = "info"
	envDebugDebug = "debug"
	envDebugWarn  = "warn"
	envDebugError = "error"
)

// Projection selects presentation encoding. Independent of Format, which
// only routes streams (stdout vs stderr). The zero value is unspecified:
// Init fills it from EVO_OUTPUT when set, otherwise human TTY/plain inference.
type projection int

const (
	// ProjectionHuman is ordinary TTY/plain inference. Non-zero so an
	// explicit Config{Projection: ProjectionHuman} wins over EVO_OUTPUT.
	projectionHuman projection = iota + 1
	// ProjectionPlain forces the durable report (no live region).
	projectionPlain
	// ProjectionJSON writes a JSONDocument at Finish.
	projectionJSON
	// ProjectionJSONL writes Event JSON Lines at Finish.
	projectionJSONL
	// ProjectionStreamJSON writes one EventJSON line at each journal append.
	projectionStreamJSON
)

func (p projection) forcesPlain() bool {
	switch p {
	case projectionPlain, projectionJSON, projectionJSONL, projectionStreamJSON:
		return true
	default:
		return false
	}
}

func (p projection) suppressesHuman() bool {
	switch p {
	case projectionJSON, projectionJSONL, projectionStreamJSON:
		return true
	default:
		return false
	}
}

var (
	lookupEnvMu sync.RWMutex
	lookupEnvFn = process.Env
)

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

// lookupEnv is the process-environment facade (same idea as processArgv0).
func lookupEnv(key string) string {
	lookupEnvMu.RLock()
	fn := lookupEnvFn
	lookupEnvMu.RUnlock()
	if fn == nil {
		return process.Env(key)
	}
	return fn(key)
}

// applyEnv fills zero Config fields from the process environment. Explicit
// Config values already set are left alone. Options-path Init skips this.
func applyEnv(c Config) Config {
	if c.projection == 0 && c.Output == 0 {
		if p, ok := parseOutputEnv(lookupEnv(envKeyOutput)); ok {
			c = selectOutputEnv(c, p)
		}
	}
	if c.projection.forcesPlain() {
		c.plain = true
	}
	if c.color == colorAuto && forceColorRequested() {
		c.color = colorAlways
	}
	if c.verbosity == verbosityNormal && lookupEnv(envKeyVerbose) == envVerboseOn {
		c.verbosity = verbosityVerbose
	}
	if c.Debug.level == levelUnset {
		if lvl, ok := parseDebugEnv(lookupEnv(envKeyDebug)); ok {
			c.Debug.level = lvl
		}
	}
	return c
}

// selectOutputEnv applies EVO_OUTPUT's projection p. json and jsonl
// select the evo.run document and evo.event stream (FormatJSON,
// FormatJSONL) when no Format was chosen: those carry every structured
// Fact, disposition and Problem, where the frozen output.v1 projection
// cannot (E-090). A Format the caller chose keeps stdout for itself, so
// there p stays the output.v1 projection on stderr.
func selectOutputEnv(c Config, p projection) Config {
	switch {
	case c.format == formatHuman && p == projectionJSON:
		c.format = formatJSON
	case c.format == formatHuman && p == projectionJSONL:
		c.format = formatJSONL
	default:
		c.projection = p
	}
	return c
}

func parseOutputEnv(v string) (projection, bool) {
	switch strings.TrimSpace(v) {
	case envOutputHuman:
		return projectionHuman, true
	case envOutputPlain:
		return projectionPlain, true
	case envOutputJSON:
		return projectionJSON, true
	case envOutputJSONL:
		return projectionJSONL, true
	default:
		return 0, false
	}
}

// forceColorRequested reports whether FORCE_COLOR asks for color. NO_COLOR
// wins over FORCE_COLOR, so it is checked first.
func forceColorRequested() bool {
	if lookupEnv(envKeyNoColor) != "" {
		return false
	}
	force := strings.TrimSpace(lookupEnv(envKeyForceColor))
	return force != "" && force != envForceColorOff
}

func parseDebugEnv(v string) (logLevel, bool) {
	switch strings.TrimSpace(v) {
	case envDebugInfo:
		return levelInfo, true
	case envDebugDebug:
		return levelDebug, true
	case envDebugWarn:
		return levelWarn, true
	case envDebugError:
		return levelError, true
	default:
		return 0, false
	}
}

// --- folded from identity.go ---

// Problem codes are stable, machine-readable Problem.Code values —
// consumers match on these instead of parsing Summary text, which is
// presentation and may be reworded.
const (
	// ProblemCodeDuplicateSiblingName marks a Task/Group/Sequence declared
	// with a name already used by another child of the same parent (§3.1).
	// get-or-create was removed for exactly this reason in 1.0: letting two
	// distinct declarations silently merge into one identity would make a
	// false "already satisfied" possible once identity drives manifest
	// reconciliation, so declaration fails instead of returning an
	// ambiguous handle.
	problemCodeDuplicateSiblingName = "duplicate-sibling-name"
	// ProblemCodeVerificationUnsatisfied marks a Task whose post-Define
	// Verify observed that the desired state was not reached (§9.1): the
	// callback ran but its intended effect could not be confirmed.
	problemCodeVerificationUnsatisfied = "verification-unsatisfied"
)

// containerNode is col's declaration in the graph, nil for the root (col == nil).
func containerNode(col *tasksState) *graph.Container {
	if col == nil {
		return nil
	}
	return col.node
}

// Key sets an advanced, refactor/rename-stable override for this Task's
// §3.1 identity, replacing the default kind+parent-key+name derivation
// within the application/workspace manifest namespace. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define, and identity is part of that configuration — a call after
// Define, or after a terminal verb settled the Task, records
// ErrKeyAfterDefine and leaves the task's key untouched. Repeating the key
// the Task already has is a no-op. A key already claimed by another Task is
// ErrDuplicateKey, the same identity-conflict error every other
// explicit-key path already reports.
func (t *Task) key(key string) *Task {
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
		o.recordMisuseFor(st.name, errKeyAfterDefine)
		return t
	}
	if o.graph.Rekey(st.node, txt.Text(key)) == graph.KeyTaken {
		o.recordMisuse(errDuplicateKey)
	}
	return t
}

// failDuplicateSiblingLocked records a real, visible Failed task carrying
// ProblemCodeDuplicateSiblingName — the truthful conclusion/exit-code path
// a duplicate declaration now takes instead of a panic or a silently
// returned existing handle. The row is named for the duplicated name
// itself, and its one Problem says what is wrong with it, so the reader
// sees `✗ t  duplicate task name` once. col is the parent container the
// duplicate was declared under, or nil for a root-level declaration.
// Callers must already hold o.mu. It returns the refusal the duplicate's
// rejected handle keeps.
func (o *Run) failDuplicateSiblingLocked(col *tasksState, kind graph.EntityKind, name string) error {
	h := o.addTaskLocked(name, col)
	rejected := fmt.Errorf("%w: %s", errDuplicateSiblingName, name)
	st := o.taskStates[h.id]
	if st == nil {
		return rejected
	}
	st.node.Rec.AppendProblems(problem{
		Code:    problemCodeDuplicateSiblingName,
		Summary: fmt.Sprintf("duplicate %s name", kind),
	})
	o.settleLocked(st, failed)
	o.recordMisuseFor(name, errDuplicateSiblingName)
	return rejected
}

// --- folded from machine.go ---

func writeMachinePresentation(w io.Writer, snap snapshot, events []recEvent, proj projection, misuse error) error {
	var body []byte
	var err error
	switch proj {
	case projectionJSON:
		body, err = wire.EncodeJSON(snap)
	case projectionJSONL:
		body, err = wire.EncodeJSONL(events)
	default:
		return misuse
	}
	if err != nil {
		err = fmt.Errorf("%w: %v", errRenderer, err)
		if misuse == nil {
			return err
		}
		return errors.Join(misuse, err)
	}
	if w != nil && len(body) > 0 {
		if _, werr := w.Write(body); werr != nil {
			werr = fmt.Errorf("%w: %v", errRenderer, werr)
			if misuse == nil {
				return werr
			}
			return errors.Join(misuse, werr)
		}
		if f, ok := w.(flusher); ok {
			_ = f.Flush()
		}
	}
	return misuse
}

// --- folded from output_lifecycle.go ---

func (o *Run) ensureOpen() error {
	if o.closed || o.finishing || o.finished {
		return errClosed
	}
	return nil
}

// Close is idempotent cleanup; best-effort Finish when needed. The first
// call tears down and returns its error; a concurrent or later call waits
// for that teardown to end and returns nil.
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
	// A Confirm still waiting for an answer would hold its caller forever
	// once the run is gone.
	for o.cancelPendingConfirmLocked(cancelCauseClose) {
	}
	needFinish := !o.finished
	o.mu.Unlock()
	if needFinish {
		_ = o.finish()
	}
	o.mu.Lock()
	o.stopSpinnerAnimatorLocked()
	o.stopResizeWatchLocked()
	o.stopPlainHeartbeatsLocked()
	o.closed = true
	o.graph.Close()
	o.mu.Unlock()
	o.graph.Cancel()
	// Finish already wrote the manifest and warned when it could not; this
	// releases the Run's exclusive manifest lock (spec §11.3) and returns
	// any write or release failure. Already committed Task records on disk
	// are unaffected — Close never rolls anything back.
	return o.manifest.Close()
}

// beginRunContext installs ctx (Run/evo.Run's own ctx parameter) as the
// parent of this run's task scopes, replacing the placeholder Init installed
// for Define/Verify calls made before any Run (see graph.Graph.BeginRunContext).
func (o *Run) beginRunContext(ctx context.Context) context.Context {
	return o.graph.BeginRunContext(ctx)
}

// Context reports the run's cancellation signal. It is cancelled when the
// run is interrupted (SIGINT/SIGTERM) and when the Output closes, so work
// that does I/O can select on it and stop instead of running on past the ^C
// that was supposed to end it. A nil Output reports a never-cancelled
// context so a caller never has to nil-check before selecting.
func (o *Run) context() context.Context {
	if o == nil {
		return context.Background()
	}
	return o.graph.Context()
}

// Conclusion returns the computed conclusion after Finish.
func (o *Run) conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok := o.rec.Conclusion(); ok {
		return asEvoConclusion(c)
	}
	snap := o.snapshotLocked()
	c := record.InferConclusion(snap)
	o.explainCancellationLocked(&c)
	record.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
	return asEvoConclusion(c)
}

// cancelCauseUser is the cause an interrupt signal records: the person at
// the terminal stopped the run.
const cancelCauseUser = "by user"

// cancelCauseClose is the cause a Close records on a Confirm still waiting.
const cancelCauseClose = "run closed"

// explainCancellationLocked names the cancellation cause on a cancelled
// conclusion. Any other outcome keeps its own Explanation untouched.
func (o *Run) explainCancellationLocked(c *record.Conclusion) {
	if c.Outcome == record.OutcomeCancelled && c.Explanation == "" {
		c.Explanation = o.graph.CancelCause()
	}
}

// --- folded from aliases.go ---

// Domain types live in internal/core / internal/txt. Engine aliases them so
// implementation files keep unqualified names; the public evo package aliases
// the same underlying types directly (not through this package).

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
	progress   = record.Progress
	factRecord = record.Fact
	recEvent   = record.Event
)

const eventSchemaVersion = record.EventSchemaVersion

type (
	snapshot        = record.Snapshot
	taskSnapshot    = record.TaskSnapshot
	taxonomyRecord  = record.TaxonomyRecord
	tasksSnapshot   = record.TasksSnapshot
	changesSnapshot = record.ChangesSnapshot
	planSnapshot    = record.PlanSnapshot
	effectRecord    = record.EffectRecord
)

const (
	exitOK        = record.ExitOK
	exitRefused   = record.ExitRefused
	exitFailed    = record.ExitFailed
	exitCancelled = record.ExitCancelled
)

type evidencePhase = record.EvidencePhase

type (
	problem        = record.Problem
	sourceLocation = record.SourceLocation
	attachment     = record.Attachment
	field          = record.Field
	commandSpec    = record.CommandSpec
	glyphProfile   = txt.GlyphProfile
)

const (
	glyphsAuto    = txt.GlyphsAuto
	glyphsUnicode = txt.GlyphsUnicode
	glyphsASCII   = txt.GlyphsASCII
)

// --- folded from option.go ---

// Option configures an Output.
type option interface {
	apply(*config)
}

type config struct {
	subject           string
	primary           io.Writer
	diagnostic        io.Writer
	result            io.Writer // domain payload (FormatData); never used for presentation
	plain             bool
	projection        projection
	noColor           bool
	width             int
	clock             timeSource
	visibilityDelay   time.Duration
	maxFrameRate      int
	strict            bool
	terminal          terminalDriver
	debugLevel        logLevel
	debugAddSource    bool
	debugPresentation debugPresentation
	debugPane         debugPaneConfig
	redactor          Redactor
	maxEntities       int
	maxEvents         int
	maxConcurrency    int
	extraWriters      []io.Writer
	verbosity         verbosity
	// stateDir/appID mirror Config.StateDir/Config.AppID (spec §11.3):
	// stateDir, when non-empty, is the exact manifest state directory;
	// appID overrides the derived application id used in the default
	// cache-dir manifest path derivation.
	stateDir string
	appID    string
	// processRunner is the facade evo.Exec spawns child processes through
	// (spec §8.4). Defaults to osProcessRunner{} in resolveConfig.
	processRunner processRunner
	// fileFS is the facade evo.File performs filesystem I/O through (spec
	// §8.2). Defaults to osFileFS{} in newOutput.
	fileFS fileFS
	// stdin is the facade Confirm reads one answer line from (default os.Stdin,
	// resolved lazily so NewWithOptions callers that skip Stdin still work).
	stdin io.Reader
	// failedExitCode overrides ExitFailed when conclusion is StateFailed.
	// Zero means use ExitFailed (2).
	failedExitCode int
	// dryRun selects ledger tense: true renders Effect/File rows as
	// [planned]/imperative, false as [changed]/past tense. Both
	// Config.DryRun and Config.Preview set it — they are one tense with two
	// announcements (see preview below).
	dryRun bool
	// preview says the planned tense above is a preview before a confirm
	// gate, not a dry run, so the opening header is the caller's subject
	// alone ("repo <path>") rather than "[dry-run] <subject>". A preview is
	// about to ask permission; labelling it a dry run tells the user nothing
	// will happen and then asks them to authorize it.
	preview bool
	// glyphs selects the state-glyph vocabulary (GlyphsAuto resolved at
	// construction to GlyphsUnicode or GlyphsASCII; see glyph.go).
	glyphs glyphProfile
	// samePrimaryAsTerminal records that the live Terminal driver was built
	// around the same underlying writer as primary (Config's default-construction
	// path: to(c.Facades.Stdout) and withTerminal(ansi-over-c.Facades.Stdout) target one stream).
	// Set only where construction knows both writers, never inferred later by
	// comparing file descriptors — see configToOptions. Finish uses it to skip
	// the CON-009 dual-stream write, which would otherwise render the
	// conclusion band twice on the one physical screen.
	samePrimaryAsTerminal bool
	// diagnosticSharesTerminal records that the Diagnostics writer resolves
	// to the same physical terminal device as the live region's Terminal
	// driver, even though the two are distinct io.Writer values (the
	// realistic default: Config{Stdout, Stderr} on an interactive shell,
	// where fd 1 and fd 2 both name the one controlling tty). Set only at
	// construction, where the real writers are known — see construct.go's
	// sameTerminalDevice detection. projectDebugRecordLocked uses it to
	// route dual-stream Debug records through the same clear-live →
	// write-durable → repaint sequencing as single-stream records, instead
	// of writing raw bytes to Diagnostics that would otherwise land
	// mid-spinner-row on the one screen both writers share.
	diagnosticSharesTerminal bool
	// dryRunHeaderText is Config.Subject's text, threaded through so a
	// dry-run's opening announcement can merge onto ONE line — "[dry-run]
	// <subject>" (fixture-repo-retire-dryrun.md) — instead of the marker and
	// the subject printing as two separate durable lines. Only Init's
	// synchronous Config{DryRun: true, Subject: "..."} path sets this before
	// construction; DeclareDryRun's later switch keeps the marker's plain
	// fallback text, since by then any Subject has already streamed as its
	// own line.
	dryRunHeaderText string
	// wireFormat is non-zero only for FormatJSON/FormatJSONL (spec §32.1) —
	// independent of the legacy projection field above, which drives the
	// pre-existing EVO_OUTPUT-selected "0.4"/"0.3" encoders. wireStream is
	// where the v2 document/event lines land (Config.Stdout); presentation
	// keeps going to primary/diagnostic exactly as FormatData routes it.
	wireFormat format
	wireStream io.Writer
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// To sets the primary human writer.
func to(w io.Writer) option {
	return optionFunc(func(c *config) { c.primary = w })
}

// Diagnostics sets the diagnostic writer for Debug history and Capture mirrors.
// When set and distinct from the primary writer (To), Debug lines are not also
// written to the human primary stream — use dual-stream for LaunchAgent /
// data-command layouts (human on stdout, diagnostics on stderr).
func withDiagnostics(w io.Writer) option {
	return optionFunc(func(c *config) { c.diagnostic = w })
}

// ResultStream sets the domain-payload writer (see Output.ResultWriter).
// Presentation never writes here. FormatData defaults this to Config.Stdout.
func resultStream(w io.Writer) option {
	return optionFunc(func(c *config) {
		if w != nil {
			c.result = w
		}
	})
}

// wireFormatOption selects the v2 machine wire encoder Finish/appendEventLocked
// write alongside ordinary human presentation (FormatJSON/FormatJSONL only —
// see configToOptions).
func wireFormatOption(f format) option {
	return optionFunc(func(c *config) { c.wireFormat = f })
}

// wireStreamOption sets where the v2 wire encoder writes (Config.Stdout).
func wireStreamOption(w io.Writer) option {
	return optionFunc(func(c *config) { c.wireStream = w })
}

// Plain forces final-report projection (no live spinner region).
// Semantic color is still emitted unless NoColor is set.
func plain() option {
	return optionFunc(func(c *config) { c.plain = true })
}

func withProjection(p projection) option {
	return optionFunc(func(c *config) { c.projection = p })
}

// NoColor disables color.
func withNoColor() option {
	return optionFunc(func(c *config) { c.noColor = true })
}

// Width sets the terminal width in columns.
func withWidth(columns int) option {
	return optionFunc(func(c *config) { c.width = columns })
}

// Clock injects a time source facade.
func withClock(ts timeSource) option {
	return optionFunc(func(c *config) { c.clock = ts })
}

// VisibilityDelay sets how long live activity must persist before the first
// interactive paint (default 80ms). Zero paints immediately. Prevents Phase→fast
// Done spinner flash (H.2). Domain TimeSource is used for the threshold.
func visibilityDelay(delay time.Duration) option {
	return optionFunc(func(c *config) { c.visibilityDelay = delay })
}

// MaxFrameRate caps interactive redraws per second.
func maxFrameRate(framesPerSecond int) option {
	return optionFunc(func(c *config) { c.maxFrameRate = framesPerSecond })
}

// Strict enables panic-on-misuse for tests.
func strict() option {
	return optionFunc(func(c *config) { c.strict = true })
}

// DryRun declares this run a dry run: evo.Effect, evo.File, and
// evo.Exec render as
// [planned] rows with imperative verbs instead of [changed] rows with
// past-tense verbs. Set once via Config.DryRun in ordinary application code;
// this Option exists for the advanced NewWithOptions surface and tests.
func dryRun() option {
	return optionFunc(func(c *config) { c.dryRun = true })
}

// dryRunHeader is Init's unexported plumbing for Config.Subject: it lets a
// dry-run's opening marker merge the subject onto its own one-line
// announcement instead of the two separate durable lines a bare Println
// would produce. Not part of the public Option surface — Config.Subject and
// Output.Subject stay the caller's only entry points.
func dryRunHeader(text string) option {
	return optionFunc(func(c *config) { c.dryRunHeaderText = text })
}

// preview declares the planned tense a preview before a confirm gate rather
// than a dry run — same skipped callbacks and same [planned] ledger, header
// without the tag. Set via Config.Preview; this Option is construction
// plumbing only.
func preview() option {
	return optionFunc(func(c *config) { c.preview = true })
}

// Stdin injects the reader Confirm reads answers from (facade rule — no
// direct os.Stdin read in Confirm's logic). Default os.Stdin.
func stdin(r io.Reader) option {
	return optionFunc(func(c *config) { c.stdin = r })
}

// Terminal injects a terminal driver (interactive projection; v0.2).
func withTerminal(driver terminalDriver) option {
	return optionFunc(func(c *config) { c.terminal = driver })
}

// withPrimarySharesTerminal marks that the just-configured Terminal driver
// writes to the same stream as primary (To). Only configToOptions calls this
// — it knows both writers at construction time, so Finish never has to guess
// stream identity from an fd comparison.
func withPrimarySharesTerminal() option {
	return optionFunc(func(c *config) { c.samePrimaryAsTerminal = true })
}

// withDiagnosticSharesTerminal marks that the Diagnostics writer resolves to
// the same physical terminal device as the live region, even when it is a
// distinct io.Writer from primary. Only configToOptions calls this — see
// config.diagnosticSharesTerminal.
func withDiagnosticSharesTerminal() option {
	return optionFunc(func(c *config) { c.diagnosticSharesTerminal = true })
}

// LogLevel is a diagnostic severity.
//
// The zero value is LevelUnset (Config resolves it to LevelInfo). Named levels
// start at LevelTrace so ordinary Config{Debug: DebugConfig{Level: LevelTrace}}
// is expressible without falling through the default path.
type logLevel int

const (
	// LevelUnset is the zero value. Config resolve maps it to LevelInfo.
	levelUnset logLevel = iota
	// LevelTrace is the most verbose journal level selectable via Config.
	levelTrace
	// LevelDebug enables Debug journal lines (and Capture MirrorToDebug).
	levelDebug
	// LevelInfo is the ordinary default (Debug journal suppressed).
	levelInfo
	// LevelWarn is reserved for future warn-threshold filtering.
	levelWarn
	// LevelError is reserved for future error-threshold filtering.
	levelError
)

// DebugLevel sets the minimum debug emission level.
// Pass LevelTrace or LevelDebug to surface Debug journal lines.
func debugLevel(level logLevel) option {
	return optionFunc(func(c *config) {
		if level == levelUnset {
			c.debugLevel = levelInfo
			return
		}
		c.debugLevel = level
	})
}

// DebugAddSource resolves each debug record's call site to a
// source=file.go:line field on human/pane/history rendering, matching
// slog.HandlerOptions.AddSource semantics. Off by default (release-gate
// round 6 finding 2): a bare pc=<uintptr> field never belongs on a human
// debug line; the raw PC still lives on LogRecord for machine consumers
// regardless of this setting.
func debugAddSource() option {
	return optionFunc(func(c *config) { c.debugAddSource = true })
}

// TerminalDriver is the exclusive owner of terminal control sequences.
type terminalDriver = terminal.Driver

// sinkReporter is implemented by a TerminalDriver that knows its own
// destination writer (terminal.ANSI, testkit's drivers). configToOptions and
// newOutput use it to DETECT whether a caller-supplied withTerminal(...) happens
// to write to a stream evident-output already knows about (primary,
// diagnostic, or either of Config's Stdout/Stderr), instead of only knowing
// that for the one construction path that builds both itself.
type sinkReporter interface {
	Sink() io.Writer
}

const (
	defaultVisibilityDelay = 80 * time.Millisecond
	defaultMaxFrameRate    = 20
	defaultWidth           = 80
	// defaultMaxEntities bounds items+tasks to prevent unbounded allocation (SEC-003).
	defaultMaxEntities = 100_000
	// defaultMaxEvents bounds the durable journal (CON-008 backpressure).
	defaultMaxEvents = 50_000
)

// MaxEntities caps total items and tasks for one Output (0 uses default).
func maxEntities(n int) option {
	return optionFunc(func(c *config) { c.maxEntities = n })
}

// MaxEvents caps durable journal events; when exceeded, oldest non-critical
// events are dropped so critical terminal events are retained (CON-008).
func maxEvents(n int) option {
	return optionFunc(func(c *config) { c.maxEvents = n })
}

func maxConcurrency(n int) option {
	return optionFunc(func(c *config) { c.maxConcurrency = n })
}

// withStateDir mirrors Config.StateDir (spec §11.3).
func withStateDir(dir string) option {
	return optionFunc(func(c *config) { c.stateDir = dir })
}

// withAppID mirrors Config.AppID (spec §11.3).
func withAppID(id string) option {
	return optionFunc(func(c *config) { c.appID = id })
}

// withProcessRunner injects the facade evo.Exec spawns every child process
// through (spec §8.4) — testkit installs a scripted fake so Exec's skip
// protocol, capture, and cancellation are provable without a real
// subprocess.
func withProcessRunner(r processRunner) option {
	return optionFunc(func(c *config) { c.processRunner = r })
}

// withFileFS injects the facade evo.File performs filesystem I/O through
// (spec §8.2) — testkit installs a fake so a reconciliation outcome (e.g.
// a chmod failure) is provable without a real disk permission.
func withFileFS(fsys fileFS) option {
	return optionFunc(func(c *config) { c.fileFS = fsys })
}

func keepLastLines_1(n int) captureOption  { return keepLastLines(n) }
func mirrorToDiagnostics_1() captureOption { return mirrorToDiagnostics() }
func mirrorToDebug_1() captureOption       { return mirrorToDebug() }
func renderPlain_1(s snapshot, opts plainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}

type (
	capture        = evidence
	systemClock_1  = systemClock
	fixedClock_1   = fixedClock
	noopRedactor_1 = noopRedactor
)

func fSPath(path string) fingerprint { return freshness.FSPath(path) }

// Test helpers reachable through the evo type alias (root export_test.go
// cannot attach methods to engine types).

func (t *Task) runForTest(cmd *process.Cmd) error {
	if t == nil || t.out == nil {
		return nil
	}
	return t.run(cmd)
}

func (t *Task) captureForTest(opts ...captureOption) *evidence {
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

func (t *Task) skippedWithErrs(reason taxonomyReason, name string, errs ...error) {
	t.recordTaxonomy(reason, name, dispositionSkip, errs)
}
func (o *Run) dropDiagnosticForTest() { o.cfg.diagnostic = nil }
func (o *Run) forceLiveVisibleForTest() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.live == nil {
		o.live = &liveEngine{}
		if live := o.liveLocked(); live != nil {
			o.live.surface = live
		}
	}
	o.live.visible = true
	o.live.paintMu.Lock()
	o.live.liveActive = true
	o.live.paintMu.Unlock()
}
func (o *Run) debugWriterForTest() io.WriteCloser           { return o.debugWriter() }
func (o *Run) declareDryRunForTest()                        { o.declareDryRun() }
func (o *Run) aboutForTest(text string)                     { o.about(text) }
func (o *Run) subjectForTest(text string)                   { o.subject(text) }
func (o *Run) slogHandlerForTest() slog.Handler             { return o.slogHandler() }
func (o *Run) nextSelfForTest(args ...string) ProblemOption { return o.nextSelf(args...) }
func (t *Task) skipForTest(reason string, args ...any) *Task {
	return t.skip(reason, args...)
}
func (o *Run) atForTest(visibility visibility) *printer { return o.at(visibility) }
func (o *Run) schedulerMaxObserved() int {
	return o.graph.MaxObserved()
}

func reasonConstrained(name string, opts ...reasonOption) taxonomyReason {
	return defaultRun().reasonGetOrCreate(name, opts...)
}

// --- folded from basis.go ---

// Fingerprint observes one Basis input's current content identity
// (spec §11.1). Implementations must not mutate the world.
type fingerprint = freshness.Fingerprint

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

// --- folded from capability.go ---

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
	cfg := config{
		width:      defaultWidth,
		debugLevel: levelInfo,
	}
	for _, o := range opts {
		if o != nil {
			o.apply(&cfg)
		}
	}
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
		if ls := asLive(cfg.terminal); ls != nil {
			p.Width = ls.Columns()
			p.Height = ls.Rows()
			p.Interactive = ls.IsInteractive() && p.Interactive
		}
	}
	return p
}

// --- folded from change_host.go ---

// This file is the engine's side of the interfaces change declares (Host):
// what the Output writes, remembers and shows on behalf of the changes a
// Task performs.

// resourceWaitingPrefix opens the live activity a Task shows while one of its
// claims waits on a conflicting holder.
const resourceWaitingPrefix = "waiting for "

// changeHost is the Output, as change asks for it.
type changeHost struct{ out *Run }

// DryRun reports whether this Output is configured for dry-run/preview
// tense — the same flag evo.Effect and evo.File render by.
func (o *Run) dryRun() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.dryRun
}

// DryRun reports whether the Run plans its changes instead of making them.
func (h changeHost) DryRun() bool { return h.out.dryRun() }

// FS is the filesystem File performs I/O through — real os calls in
// ordinary use, a scripted fake under test (facade rule).
func (h changeHost) FS() fs.FS {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	return h.out.cfg.fileFS
}

// taskLedger is one Task's ledger section, resolved once.
type taskLedger struct {
	out    *Run
	taskID string
	target ledgerTarget
}

// Applies reports whether the ledger records changes made, not planned.
func (l taskLedger) Applies() bool { return l.target.tense == tenseChanged }

// Record adds e to the Task's section.
func (l taskLedger) Record(e record.LedgerEntry) {
	l.out.recordResolvedEntry(l.taskID, l.target, e)
}

// Ledger resolves id's ledger section; see Output.resolveLedgerTarget.
func (h changeHost) Ledger(id record.TaskID) (change.Ledger, error) {
	target, err := h.out.resolveLedgerTarget(string(id))
	if err != nil {
		return nil, err
	}
	return taskLedger{out: h.out, taskID: string(id), target: target}, nil
}

// AttachVerification adds the per-attribute evidence a reconcile produced to
// id's record; see Output.attachVerificationLocked.
func (h changeHost) AttachVerification(id record.TaskID, details []record.VerificationDetail) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.attachVerificationLocked(string(id), details)
}

// Manifest opens this Run's manifest on first use; see Output.manifestFor.
func (h changeHost) Manifest(ctx context.Context) (*freshness.ManifestStore, error) {
	return h.out.manifestFor(ctx)
}

// NoteManifestWarning surfaces a safe cache-miss warning once per Run.
func (h changeHost) NoteManifestWarning(warning error) { h.out.emitManifestWarningOnce(warning) }

// TaskManifest is id's stable manifest key and its next operation's ordinal.
func (h changeHost) TaskManifest(id record.TaskID) (key string, ordinal int, ok bool) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	return h.out.taskManifestKeyLocked(string(id))
}

// AppendOperation records rec as id's next pending operation.
func (h changeHost) AppendOperation(id record.TaskID, rec freshness.OperationRecord) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.appendManifestOperationLocked(string(id), rec)
}

// PlanOperation holds id's next operation's place without a record.
func (h changeHost) PlanOperation(id record.TaskID) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.planManifestOperationLocked(string(id))
}

// ShowWaiting makes id's row read "waiting for <label>" and returns the
// function that restores what it showed before.
func (h changeHost) ShowWaiting(id record.TaskID, label string) (restore func()) {
	o, taskID := h.out, string(id)
	text := txt.Text(resourceWaitingPrefix + label)
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil || record.IsTerminalTask(st.node.Rec.State()) {
		return nil
	}
	prior := st.node.Rec.Phase()
	o.setLiveOnlyPhaseLocked(st, text)
	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		st := o.taskStates[taskID]
		if st == nil || record.IsTerminalTask(st.node.Rec.State()) || st.node.Rec.Phase() != text {
			return
		}
		o.setLiveOnlyPhaseLocked(st, prior)
	}
}

// --- folded from change_observe.go ---

// Observe narrates one step of a tracked operation as the §38 "evo.event"
// line it has always been: change speaks of subjects and steps, the wire
// vocabulary stays here.
func (h changeHost) Observe(ob change.Observation) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.emitWireEventLocked(observationEvent(ob.Kind), string(ob.Task), observationPayload(ob))
}

// observationEvent is the §38 event type for a step of a tracked operation.
func observationEvent(kind change.ObservationKind) string {
	switch kind {
	case change.OperationStarted:
		return wire.EventOperationStarted
	case change.OperationSkippedCurrent:
		return wire.EventOperationSkippedCurrent
	case change.OperationFinished:
		return wire.EventOperationFinished
	case change.ResourceObserved:
		return wire.EventTrackedResourceObserved
	default:
		return wire.EventBasisFingerprinted
	}
}

// observationPayload is the §38 payload for ob: the subject's kind and name
// under the key its kind has always used, then the fields its step carries.
// A File's basis.fingerprinted line has never named its kind, and still does
// not.
func observationPayload(ob change.Observation) map[string]any {
	kind, nameKey := wire.PayloadKindFile, "path"
	switch ob.Subject.Kind {
	case change.SubjectExec:
		kind, nameKey = wire.PayloadKindExec, "executable"
	case change.SubjectExecOutput:
		kind = wire.PayloadKindExecOutput
	}
	payload := map[string]any{nameKey: ob.Subject.Name}
	if ob.Kind != change.BasisFingerprinted || ob.Subject.Kind != change.SubjectFile {
		payload["kind"] = kind
	}
	switch ob.Kind {
	case change.OperationStarted, change.OperationSkippedCurrent:
		if ob.Reason != "" {
			payload["reason"] = ob.Reason
		}
	case change.OperationFinished:
		payload["changed"] = ob.Changed
	case change.ResourceObserved:
		payload["exists"] = ob.Exists
	case change.BasisFingerprinted:
		payload["count"] = ob.Count
	}
	return payload
}

// --- folded from conjugate.go ---

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
	return txt.Pluralize(quantity, singular)
}

// --- folded from evidence.go ---

const (
	defaultCaptureLines = process.DefaultCaptureLines
	defaultCaptureBytes = process.DefaultCaptureBytes
	maxCaptureLineLen   = 4096
	maxCaptureChunk     = process.MaxCaptureChunk
	truncationMarker    = process.TruncationMarker
)

// CaptureStream identifies which process stream a line came from.
type captureStream = process.Stream

const (
	captureStreamCombined = process.StreamCombined
	captureStreamStdout   = process.StreamStdout
	captureStreamStderr   = process.StreamStderr
)

// evidence is the retained/redacted process-output sink owned by a Task
// (preferred) or Output. The bounded ring lives in process; this type owns
// sanitization, redaction, mirroring, and Task attachment.
type evidence struct {
	out      *Run
	taskID   string
	taskName string
	ring     *process.Ring
	maxLines int
	maxBytes int

	mirrorDiag  bool
	mirrorDebug bool
	stream      captureStream
	parent      *evidence
	onLine      func(text string)
}

// CaptureOption configures Capture.
type captureOption interface {
	applyCapture(*evidence)
}

type captureOptionFunc func(*evidence)

func (f captureOptionFunc) applyCapture(c *evidence) { f(c) }

func keepLastLines(n int) captureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

func mirrorToDiagnostics() captureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDiag = true })
}

func mirrorToDebug() captureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDebug = true })
}

func activityFeed(fn func(text string)) captureOption {
	return captureOptionFunc(func(c *evidence) { c.onLine = fn })
}

func (t *Task) capture(opts ...captureOption) *evidence {
	if t == nil || t.out == nil {
		return newCapture(nil, "", "", opts...)
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskStates[t.id]
	if st == nil {
		return newCapture(t.out, t.id, "", opts...)
	}
	if st.evidence == nil {
		st.evidence = newCapture(t.out, t.id, st.name, opts...)
	}
	return st.evidence
}

func newCapture(out *Run, taskID, taskName string, opts ...captureOption) *evidence {
	c := &evidence{
		out:         out,
		taskID:      taskID,
		taskName:    taskName,
		maxLines:    defaultCaptureLines,
		maxBytes:    defaultCaptureBytes,
		mirrorDiag:  false,
		mirrorDebug: false,
		stream:      captureStreamCombined,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(c)
		}
	}
	c.ring = process.NewRing(c.maxLines, c.maxBytes)
	c.ring.SetOnAppend(func(line string) {
		if c.onLine != nil {
			c.onLine(line)
		}
		if c.out != nil && (c.mirrorDiag || c.mirrorDebug) {
			c.out.mirrorCaptureLine(c.mirrorDiag, c.mirrorDebug, c.taskName, line)
		}
	})
	return c
}

func (c *evidence) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: captureStreamStdout, ring: c.ring}
}

func (c *evidence) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: captureStreamStderr, ring: c.ring}
}

func (c *evidence) Write(p []byte) (int, error) {
	root := c.root()
	if root == nil || root.ring == nil {
		return len(p), nil
	}
	stream := captureStreamCombined
	if c.parent != nil {
		stream = c.stream
	}
	return root.ring.Write(p, stream, root.normalizeCaptureLine)
}

func (c *evidence) Close() error {
	root := c.root()
	if root == nil || root.ring == nil {
		return nil
	}
	if c.parent != nil {
		return root.ring.Close([]process.Stream{c.stream}, root.normalizeCaptureLine)
	}
	return root.ring.Close([]process.Stream{
		captureStreamCombined, captureStreamStdout, captureStreamStderr,
	}, root.normalizeCaptureLine)
}

func (c *evidence) root() *evidence {
	if c == nil {
		return nil
	}
	if c.parent != nil {
		return c.parent
	}
	return c
}

func (c *evidence) Text() string {
	lines, truncated := c.snapshotTexts(captureStreamCombined, 0)
	return process.JoinLines(lines, truncated)
}

func (c *evidence) streamText(stream captureStream) string {
	lines, _ := c.snapshotTexts(stream, 0)
	return strings.Join(lines, "\n")
}

func (c *evidence) wasTruncated() bool {
	root := c.root()
	if root == nil || root.ring == nil {
		return false
	}
	return root.ring.Truncated()
}

func (c *evidence) Empty() bool {
	root := c.root()
	if root == nil || root.ring == nil {
		return true
	}
	return root.ring.Empty()
}

func (c *evidence) DetailTail() ProblemOption {
	return record.WithEvidenceTail(c.detailText)
}

func (c *evidence) detailText() string {
	root := c.root()
	if root == nil || root.ring == nil {
		return ""
	}
	prefer := captureStreamCombined
	if c.parent != nil {
		prefer = c.stream
	} else if root.ring.PreferStderr() || root.ring.HasStream(captureStreamStderr) {
		prefer = captureStreamStderr
	}
	texts, truncated := root.ring.Texts(prefer, 0, root.normalizeCaptureLine)
	if len(texts) == 0 {
		texts, truncated = root.ring.Texts(captureStreamCombined, 0, root.normalizeCaptureLine)
	}
	if len(texts) == 0 {
		return ""
	}
	var b strings.Builder
	if truncated {
		b.WriteString(truncationMarker)
		b.WriteByte('\n')
	}
	if len(texts) > 1 {
		fmt.Fprintf(&b, "Last %d lines:\n", len(texts))
	}
	b.WriteString(strings.Join(texts, "\n"))
	return b.String()
}

func (c *evidence) snapshotTexts(stream captureStream, limit int) ([]string, bool) {
	root := c.root()
	if root == nil || root.ring == nil {
		return nil, false
	}
	return root.ring.Texts(stream, limit, root.normalizeCaptureLine)
}

func (c *evidence) normalizeCaptureLine(line string) string {
	cut := len(line) > maxCaptureChunk
	if cut {
		line = line[:maxCaptureChunk]
	}
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = record.SanitizeText(line)
	if c.out != nil {
		line = c.out.redactString(line)
	}
	line = record.TruncateUTF8(line, maxCaptureLineLen, "…")
	if cut && !strings.HasSuffix(line, "…") {
		line += "…"
	}
	return line
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
	interactive := false
	if live := o.liveLocked(); live != nil {
		interactive = live.IsInteractive() && !o.cfg.plain
	}
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

var _ io.WriteCloser = (*evidence)(nil)

// --- folded from execution.go ---

// Define lives in verify.go, alongside its Verify-aware execution wiring
// (runDefine) — both are one concern (§7, §9.1). The graph owns the executor:
// when a callback starts, how many run at once, and what a stall or an
// interrupt does to the Tasks behind it (see graph.Graph.Kick).

func (t *Task) submitWork(run func() error) {
	if t == nil || t.out == nil {
		return
	}
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	if st == nil {
		o.mu.Unlock()
		return
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		o.mu.Unlock()
		return
	}
	// §48: a predecessor that already failed settles this Task NotStarted
	// right here, under the same lock, and so does an interrupt.
	switch o.graph.Submit(st.node, o.workOf(st, run)) {
	case graph.AlreadySettled:
		o.recordMisuseFor(st.name, errAlreadyResolved)
		o.mu.Unlock()
		return
	case graph.AlreadySubmitted:
		o.recordMisuse(errInvalidConfig)
		o.mu.Unlock()
		return
	}
	o.followRecordLocked()
	o.mu.Unlock()
	o.graph.Kick()
}

// workOf is what the graph runs for st: the callback, and the engine's own
// steps around it.
func (o *Run) workOf(st *taskState, run func() error) graph.Work {
	return graph.Work{
		Started:  func() bool { return o.taskStarted(st) },
		Run:      run,
		Observed: func(err error) { o.resolveObserved(st, err) },
		Panicked: st.handle.failScheduled,
	}
}

// taskStarted is the render work a claimed Task owes: the eligibility recEvent,
// the move to Running, the snapshot version and a forced paint, within the
// live render budget, since a start is the spinner FP-005 requires before the
// check. It reports whether the Task began: false when an interrupt settled
// it between its claim and this step, so the scheduler skips its callback.
func (o *Run) taskStarted(st *taskState) bool {
	if st.isGate() {
		return true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
	if record.IsTerminalTask(st.node.Rec.State()) {
		// An interrupt settled the Task between its claim and this step: it
		// never started, so it is neither eligible nor started.
		return false
	}
	o.emitWireEventLocked(wire.EventTaskEligible, st.id, nil)
	if st.node.Rec.State() == pending {
		o.promoteRunningLocked(st)
	} else {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.signalLiveLocked(true)
	return true
}

// resolveObserved commits the task's outcome from what the callback actually
// returned, ratifying or discarding the caller's proposal (see
// Task.finish). A ratified proposal supplies the row's summary — the
// caller's own words, now backed by an observation. A returned error
// replaces the proposal: it was never a resolution, so it earns no
// "resolve each task once" misuse line (E-113).
func (o *Run) resolveObserved(st *taskState, err error) {
	observation := o.graph.Observe(st.node, err)
	switch observation.Kind {
	case graph.ObservedFailure:
		st.handle.settleEnding(err)
	case graph.ObservedProposal:
		proposal := observation.Proposal
		st.handle.resolveScheduled(proposal.State, proposal.Summary, proposal.Problems)
	default:
		st.handle.succeedScheduled()
	}
}

// drainScheduler runs the queue to empty.
func (o *Run) drainScheduler() { o.graph.Drain() }

// --- folded from file_manifest.go ---

// emitManifestWarningOnce surfaces a manifest Store's safe cache-miss
// warning (corrupt/unknown prior manifest, spec §11.3) as a run-scoped Fact
// exactly once per Run — never as an error, since a miss is always safe to
// continue past.
func (o *Run) emitManifestWarningOnce(w error) {
	if w == nil || !o.manifest.FirstMissWarning() {
		return
	}
	o.Fact("manifest", w.Error())
}

// taskManifestKeyLocked returns taskID's stable manifest Task key (§3.1's
// stableKey, already computed at declaration) and the next pending
// operation ordinal within it. Callers must already hold o.mu.
func (o *Run) taskManifestKeyLocked(taskID string) (key string, ordinal int, ok bool) {
	st := o.taskStates[taskID]
	if st == nil {
		return "", 0, false
	}
	return st.ManifestKey(), o.taskFreshness.OperationCount(st.TaskID()), true
}

// appendManifestOperationLocked records rec as taskID's next pending
// operation, committed only if/when the Task itself settles Done (spec
// §8.2/§11.3). Callers must already hold o.mu.
func (o *Run) appendManifestOperationLocked(taskID string, rec freshness.OperationRecord) {
	if st := o.taskStates[taskID]; st != nil {
		o.taskFreshness.AppendOperation(st.TaskID(), rec)
	}
}

// planManifestOperationLocked notes that taskID's next operation was found
// stale and, in a planned Run, left unapplied; it holds its place in the
// Task's order without a record. Callers must already hold o.mu.
func (o *Run) planManifestOperationLocked(taskID string) {
	if st := o.taskStates[taskID]; st != nil {
		o.taskFreshness.PlanOperation(st.TaskID())
	}
}

// commitManifestTaskLocked persists taskID's accumulated operations as this
// Run's truth (spec §11.3) once the Task has settled Done; see
// freshness.TaskTable.Commit. Callers must already hold o.mu.
func (o *Run) commitManifestTaskLocked(ctx context.Context, taskID string) {
	st := o.taskStates[taskID]
	if st == nil {
		return
	}
	commit := o.taskFreshness.Commit(ctx, o.manifest, st, o.runModeLocked())
	if commit.Err != nil {
		o.warnManifestUnsavedLocked(commit.Err)
		return
	}
	if commit.Written {
		o.emitWireEventLocked(wire.EventManifestTaskCommitted, taskID, map[string]any{
			"operations": commit.Operations,
		})
	}
}

// saveManifest waits until every record this Run committed or staged is
// on disk (see freshness.ManifestSession.Save) and warns on the run when it
// could not be: the next run re-executes work this one did, and the reader
// must know why.
func (o *Run) saveManifest() {
	if err := o.manifest.Save(); err != nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		if !o.finished {
			o.warnManifestUnsavedLocked(err)
		}
	}
}

// warnManifestUnsavedLocked states once per run that the manifest could
// not be saved. Callers must already hold o.mu.
func (o *Run) warnManifestUnsavedLocked(err error) {
	if !o.manifest.FirstUnsavedWarning() {
		return
	}
	o.warnLocked(record.ApplyProblemOptions(txt.Text("manifest not saved: "+err.Error()), nil))
}

// --- folded from freshness_bridge.go ---

// This file is the engine's side of the interfaces freshness declares: the
// manifest session's location and the manifest it opens.

// manifestConfig names the manifest this Run reads: the configured
// application id and state directory, scoped to the Run's workspace.
func (o *Run) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: o.changes.Workspace()}
}

// manifestFor returns this Run's manifest Store, opening it on first use
// (spec §11.3). A manifest miss or open failure degrades this Run to
// live-filesystem-only File behavior; see freshness.ManifestSession.Store.
func (o *Run) manifestFor(ctx context.Context) (*freshness.ManifestStore, error) {
	store, err := o.manifest.Store(ctx)
	if errors.Is(err, freshness.ErrManifestSessionClosed) {
		return nil, errClosed
	}
	return store, err
}

// runModeLocked is whether this Run applies its work or only plans it, as
// freshness asks. Callers must already hold o.mu.
func (o *Run) runModeLocked() freshness.RunMode {
	if o.cfg.dryRun {
		return freshness.RunPlans
	}
	return freshness.RunApplies
}

// --- folded from freshness_task.go ---

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FingerprintBasis.
type basisSource = freshness.BasisSource

// FingerprintBasis observes a Fingerprint (FSPath, Value, App).
func fingerprintBasis(f fingerprint) basisSource { return freshness.FingerprintBasis(f) }

// basisIsCurrent judges this Task's Basis as it starts (see
// freshness.TaskTable.JudgeBasis) and narrates the verdict. A Task with no
// Basis is never current.
func (t *Task) basisIsCurrent(ctx context.Context) (bool, error) {
	o := t.out
	o.mu.Lock()
	st, mode := o.taskStates[t.id], o.runModeLocked()
	o.mu.Unlock()
	if st == nil {
		return false, nil
	}
	judgement, err := o.taskFreshness.JudgeBasis(ctx, o.manifestFor, st, mode)
	if err != nil || !judgement.Declared {
		return false, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventBasisFingerprinted, t.id, map[string]any{
		"count": len(judgement.Observed), "current": judgement.Current,
	})
	return judgement.Current, nil
}

// --- folded from interrupt.go ---

// interrupt stops the run at the first signal. The graph owns the order that
// leaves the ledger honest (see graph.Graph.Interrupt); the engine's part is
// putting the active rows into the state the reader must see.
func (o *Run) interrupt(reason string) {
	if o == nil {
		return
	}
	o.graph.Interrupt(cancelCauseUser, func() { o.cancelActive(reason) })
}

// cancelActive cancels the currently running task, or the output itself when
// no task is running, so an interrupt always leaves a typed Cancelled state.
//
// A pending Confirm gate takes priority over the generic task scan below: a
// gate holds sole control of the run (Confirm suspends the live region and
// blocks on stdin) and its abort channel — not Task.Cancel — is what
// unblocks the stdin read. Since a Confirm gate is an ordinary Task while
// its answer is pending, the generic Pending-task fallback would otherwise
// resolve it to Cancelled without ever closing that channel, leaving
// readConfirmLine blocked forever.
func (o *Run) cancelActive(reason string) {
	o.mu.Lock()
	if o.cancelPendingConfirmLocked(reason) {
		o.mu.Unlock()
		return
	}
	running := make([]*Task, 0, len(o.tasks))
	for _, t := range o.tasks {
		if t.node.Rec.State() == record.Running {
			running = append(running, t.handle)
		}
	}
	if len(running) > 0 {
		o.mu.Unlock()
		for _, t := range running {
			t.cancel(reason)
		}
		return
	}
	// Nothing has reached Running yet: the earliest-declared Pending task is
	// the one about to run next (evo-rec.md "one Running child" — pending
	// siblings are named and idle, waiting their turn), so an interrupt
	// before any evidence still cancels that task rather than falling
	// through to Output-level cancel.
	var active *Task
	for _, t := range o.tasks {
		if t.node.Rec.State() == pending {
			active = t.handle
			break
		}
	}
	if active != nil {
		o.mu.Unlock()
		active.cancel(reason)
		return
	}
	o.mu.Unlock()
	o.cancel(reason)
}

// cancelPendingConfirmLocked cancels one pending Confirm gate, if any, so ^C
// at a "[y/N]" prompt unblocks Confirm's stdin read and resolves the gate as
// Cancelled — never Blocked "declined" (a human "n" and an interrupt are
// distinct outcomes). Reports whether a gate was cancelled.
func (o *Run) cancelPendingConfirmLocked(reason string) bool {
	id, ok := o.graph.AbortPending()
	if !ok {
		return false
	}
	if st := o.taskStates[id]; st != nil && !record.IsTerminalTask(st.node.Rec.State()) {
		st.node.Rec.SetSummary(reason)
		o.settleLocked(st, cancelled)
		o.commitResolvedTaskLocked(id)
	}
	return true
}

// --- folded from misuse.go ---

// misuseGlyph prefixes the one required misuse line the same way the
// existing "! already mutated: ..." row does (writeAlreadyMutated) — "!" is
// attention-only per evo-rec.md's glyph vocabulary, and a misuse line always
// demands attention.
const misuseGlyph = "!"

func (o *Run) recordMisuse(err error) {
	o.misuse.Record(err)
}

func (o *Run) recordMisuseFor(subject string, err error) {
	o.misuse.RecordMisuseFor(subject, err)
}

func (o *Run) firstMisuse() error {
	return o.misuse.First()
}

// appendMisuseLineLocked renders the one required line for the first
// recorded misuse — an exit code may never disagree with everything the
// caller saw printed (beginner-1). The line is the recorder's corrective
// sentence for the recorded sentinel, never the raw "evo: ..." sentinel
// text (release-gate round 4 finding 2): machine detail stays in
// JSON/debug, the human stream gets told what to do next.
func (o *Run) appendMisuseLineLocked() {
	hint := o.misuse.Hint()
	glyph := txt.StyleGlyph(misuseGlyph, txt.SGRYellow, !o.cfg.noColor)
	o.rec.AppendLine(fmt.Sprintf("%s  %s", glyph, hint))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Fail/Block/Warn/Cancel/Skipped) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Run) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	o.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}

// Err returns the first recorded misuse error, if any.
func (o *Run) err() error {
	return o.firstMisuse()
}

// --- folded from position_set.go ---

// positionSet is a set of non-negative positions that yields its members
// in ascending order without visiting the gaps: a tree of 64-bit words
// where each bit of a level marks a non-empty word of the level below.
// Adding, removing, and finding the next member cost the tree's height,
// so the first k of a set of 16000 cost k steps, not 16000.
type positionSet struct {
	levels [][]uint64
	size   int
}

// len is how many positions the set holds.
func (s *positionSet) len() int { return s.size }

// has reports whether i is a member.
func (s *positionSet) has(i int) bool {
	w := i >> 6
	return len(s.levels) > 0 && w < len(s.levels[0]) && s.levels[0][w]&(1<<(i&63)) != 0
}

// add makes i a member.
func (s *positionSet) add(i int) {
	if s.has(i) {
		return
	}
	s.size++
	s.reach(i)
	for l := range s.levels {
		w := i >> 6
		wasEmpty := s.levels[l][w] == 0
		s.levels[l][w] |= 1 << (i & 63)
		if !wasEmpty {
			return
		}
		i = w
	}
}

// reach grows the tree until it can hold i: every level has the word i
// falls in, and the top level is one word, so add has a root to stop at.
func (s *positionSet) reach(i int) {
	for l := 0; ; l++ {
		if l == len(s.levels) {
			s.levels = append(s.levels, nil)
			s.seed(l)
		}
		for len(s.levels[l]) <= i>>6 {
			s.levels[l] = append(s.levels[l], 0)
		}
		if l == len(s.levels)-1 && len(s.levels[l]) == 1 {
			return
		}
		i >>= 6
	}
}

// seed marks in a new level l the non-empty words of level l-1.
func (s *positionSet) seed(l int) {
	if l == 0 {
		return
	}
	for w, word := range s.levels[l-1] {
		if word == 0 {
			continue
		}
		for len(s.levels[l]) <= w>>6 {
			s.levels[l] = append(s.levels[l], 0)
		}
		s.levels[l][w>>6] |= 1 << (w & 63)
	}
}

// remove makes i a non-member.
func (s *positionSet) remove(i int) {
	if !s.has(i) {
		return
	}
	s.size--
	for l := 0; l < len(s.levels); l++ {
		w := i >> 6
		s.levels[l][w] &^= 1 << (i & 63)
		if s.levels[l][w] != 0 {
			return
		}
		i = w
	}
}

// next is the smallest member at or above from, or -1.
func (s *positionSet) next(from int) int { return s.nextAt(0, from) }

func (s *positionSet) nextAt(level, from int) int {
	if level >= len(s.levels) {
		return -1
	}
	w := from >> 6
	if w >= len(s.levels[level]) {
		return -1
	}
	if m := s.levels[level][w] & (^uint64(0) << (from & 63)); m != 0 {
		return w<<6 + bits.TrailingZeros64(m)
	}
	nw := s.nextAt(level+1, w+1)
	if nw < 0 {
		return -1
	}
	return nw<<6 + bits.TrailingZeros64(s.levels[level][nw])
}

// appendFirst appends the k smallest members, ascending, to dst.
func (s *positionSet) appendFirst(dst []int, k int) []int {
	for p, n := s.next(0), 0; p >= 0 && n < k; p, n = s.next(p+1), n+1 {
		dst = append(dst, p)
	}
	return dst
}

// each calls fn on every member, ascending.
func (s *positionSet) each(fn func(int)) {
	for p := s.next(0); p >= 0; p = s.next(p + 1) {
		fn(p)
	}
}

// --- folded from redact.go ---

// Redactor redacts sensitive values before journal, Capture retention, and human rendering.
type Redactor = process.Redactor

// noopRedactor leaves strings unchanged.
type noopRedactor = process.NoopRedactor

// Redact injects a redactor (Debug fields, Capture lines, problem detail paths).
func redact(r Redactor) option {
	return optionFunc(func(c *config) {
		if r != nil {
			c.redactor = r
		}
	})
}

// redactString applies the configured redactor.
//
// It takes no lock: the redactor is an Option, fixed when the Output is
// constructed and never reassigned, and the constructor's return is what
// publishes it. Reading it under Output.mu deadlocked the one path that
// needs it most — a scheduled failure auto-attaching its evidence tail,
// where resolve already holds that lock — so the lock bought nothing and
// cost the whole run.
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

// --- folded from run_annotations.go ---

// Fact records a discovered name/value annotation on the default instance's
// run itself, not on any one task — evo.Fact's package-level form. See
// Output.Fact.
func fact(name, value string) {
	defaultRun().Fact(name, value)
}

// Fact accumulates a run-scoped discovered name/value annotation (P8
// symmetry with Task.Fact) — information about the run, fire-and-
// forget: it both renders a durable dim "name  value" line immediately
// (the same "act now" contract Println has) and stores the annotation for
// the structured Snapshot/JSON views. A nil Output is safe and records
// nothing.
func (o *Run) Fact(name, value string) {
	if o == nil {
		return
	}
	f := record.SanitizeFact(factRecord{Name: txt.Text(name), Value: txt.Text(value)})
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.rec.RecordRunFact(f)
	o.bumpLocked()
	o.emitWireEventLocked(wire.EventFactRecorded, "", wire.ToFactDoc(f).EventPayload())
	o.writeDurableTextLocked(txt.Dim(f.Name+"  "+f.Value, !o.cfg.noColor) + "\n")
}

// warnLocked records p as a run-scoped warning and renders it. Callers
// must already hold o.mu. Public Warn was removed in 1.1; this is the
// engine's own path (a failed manifest flush) onto the same projection
// Task.Problem(..., Severity(SeverityWarning)) uses for the
// conclusion's warned modifier.
func (o *Run) warnLocked(p problem) {
	o.rec.RecordRunWarning(p)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "run.warned", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(o.cfg.glyphs), txt.SGRYellow, !o.cfg.noColor)
	o.writeDurableTextLocked(glyph + " " + p.Summary + "\n")
}

// --- folded from run_signal.go ---

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (wired to SIGINT/SIGTERM by those
// entrypoints) in place of the pre-v0.6 no-context func() error form.
type RunFunc func(context.Context, *Run) error

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

// --- folded from verify.go ---

// verifyEvidenceSource names Verify as the evidence source recorded on a
// TaskSnapshot's Evidence phases (§30).
const verifyEvidenceSource = "verify"

// operationsEvidenceSource names Evo-native tracked operations (evo.File,
// evo.Exec in a later increment) as the after-Define evidence source (§9.2:
// "derive post-definition Evidence when modeled proof exists") — used only
// when the Task registered no explicit Verify, so the two sources never
// compete for the same phase.
const operationsEvidenceSource = "operations"

// errVerificationUnsatisfied is runDefine's internal signal that a
// post-Define Verify observed the desired state was not reached — the task
// is already failed (with ProblemCodeVerificationUnsatisfied) by the time
// this is returned; its only remaining job is to read non-nil to
// executeWork's generic "already resolved by the callback" branch (see
// Task.finish/resolveScheduled) so sequence followers still cascade.
var errVerificationUnsatisfied = errors.New("evo: postcondition not satisfied")

// Verify registers an advanced current-state observation check (§9.1),
// ANDed with any previously registered check on this Task. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define — a call after Define is ignored and recorded as misuse.
//
// Define's own execution wiring (see runDefine) runs every registered
// Verify twice: once before the callback (with the Task already Running) —
// all true skips the callback entirely and settles the Task Satisfied;
// any false enters the callback; an
// observation error fails the Task without entering the callback — and
// once after a successful callback, where any false fails the Task with
// ProblemCodeVerificationUnsatisfied and an observation error fails it
// plainly. Neither check commits a success record on its own; only a fully
// satisfied pass (pre- or post-) does. The after-check is skipped in two
// cases only (hasPostStateToVerify): Define resolved the Task itself
// (Block, or Kept/Skipped with no Effect committed first), or a dry run or
// preview skipped an Effect Define planned, so the observed state is the
// one before the plan. A planned run whose Define planned nothing is
// checked like a real one.
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
		o.recordMisuseFor(st.name, errInvalidConfig)
		return t
	}
	o.taskFreshness.AddVerifier(st.TaskID(), freshness.Verifier(fn))
	return t
}

// Define freezes this Task's dependency/verification configuration and
// submits fn to the scheduler (§7): Define itself returns immediately after
// submission, before fn ever runs; the scheduler starts fn as soon as the
// Task is eligible; fn's error becomes the Task's outcome (see runDefine for
// the Verify-aware resolution wiring); evo.Run/evo.Main wait for every
// submitted Task before returning their Result.
//
// Define returns the same *Task (ZYS-849 Decisions) so the common
// single-Task shape can be written `return task.Define(fn).Wait()`. This is
// fluent sugar only — it does not change Define's asynchronous submission:
// fn still runs on the scheduler, not inline before Define returns.
//
// A second Define on the same Task is misuse (submitWork's own
// already-submitted guard) — configuration freezes once.
func (t *Task) Define(fn func(context.Context) error, opts ...DefineOption) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	if fn == nil {
		o.recordMisuse(errInvalidConfig)
		return t
	}
	o.mu.Lock()
	st := o.taskStates[t.id]
	if st == nil {
		o.recordMisuse(t.rejected)
		o.mu.Unlock()
		return t
	}
	verifiers := o.taskFreshness.Verifiers(st.TaskID())
	o.mu.Unlock()

	var cfg defineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	fn = o.cleanStops.wrap(cfg.cleanStopSignals, fn)
	t.submitWork(func() error { return t.runDefine(verifiers, fn) })
	return t
}

// runDefine is Define's Verify-aware execution wiring (§7, §9.1, §29/§30).
// It resolves the Task itself on every path except "callback returned an
// error" and "callback returned success with nothing left to check" — those
// two fall through to the scheduler's own generic resolution
// (executeWork/resolveObserved) via passthroughCallbackOutcome, exactly as a
// Verify-less Define always has.
func (t *Task) runDefine(verifiers []freshness.Verifier, fn func(context.Context) error) error {
	o := t.out
	scope := graph.NewScope(o.graph, record.TaskID(t.id), o.changes)

	if current, basisErr := t.basisIsCurrent(o.context()); basisErr != nil {
		t.failScheduled(basisErr.Error())
		return passthroughCallbackOutcome(basisErr)
	} else if current {
		t.satisfyScheduled()
		return nil
	}

	if len(verifiers) > 0 {
		allSatisfied, obsErr := evaluateVerifiers(graph.WithScope(o.context(), scope), o, t.id, verifiers)
		if obsErr != nil {
			o.recordEvidencePhase(t.id, evidencePhaseBefore, true, false)
			t.failScheduled(obsErr.Error())
			return passthroughCallbackOutcome(obsErr)
		}
		o.recordEvidencePhase(t.id, evidencePhaseBefore, true, allSatisfied)
		if allSatisfied {
			t.satisfyScheduled()
			return nil
		}
	}

	o.mu.Lock()
	o.emitWireEventLocked(wire.EventDefinitionStarted, t.id, nil)
	o.mu.Unlock()
	callbackErr := fn(graph.WithScope(o.context(), scope))
	o.mu.Lock()
	scope.Close()
	o.emitWireEventLocked(wire.EventDefinitionFinished, t.id, map[string]any{"failed": callbackErr != nil})
	o.mu.Unlock()
	if callbackErr != nil {
		return passthroughCallbackOutcome(callbackErr)
	}

	if err := t.checkAfterDefine(verifiers, scope); err != nil {
		return err
	}
	o.markDefineRan(t.id)
	return nil
}

// checkAfterDefine is runDefine's post-callback evidence step: every
// registered Verify must now hold, or, with none, tracked operations
// supply the After phase. It returns the carrier error when the Task
// already failed.
func (t *Task) checkAfterDefine(verifiers []freshness.Verifier, scope *graph.Scope[*change.Run]) error {
	o := t.out
	switch {
	case !t.hasPostStateToVerify():
		// The After phase stays unevaluated.
	case len(verifiers) > 0:
		allSatisfied, obsErr := evaluateVerifiers(graph.WithScope(o.context(), scope), o, t.id, verifiers)
		if obsErr != nil {
			o.recordEvidencePhase(t.id, evidencePhaseAfter, true, false)
			t.failScheduled(obsErr.Error())
			return passthroughCallbackOutcome(obsErr)
		}
		o.recordEvidencePhase(t.id, evidencePhaseAfter, true, allSatisfied)
		if !allSatisfied {
			t.failScheduledWithCode(problemCodeVerificationUnsatisfied, "postcondition not satisfied")
			return passthroughCallbackOutcome(errVerificationUnsatisfied)
		}
	default:
		// No explicit Verify: derive post-Define Evidence from Evo-native
		// tracked operations when Define recorded any (§9.2). Every
		// operation evo.File appended to operation records already re-inspected
		// and confirmed its own managed attributes before returning success
		// (§8.2's "re-inspect / verify every managed attribute" step) — a
		// mismatch there would have made callbackErr non-nil, so reaching
		// here with a non-empty set of operation records means every tracked
		// operation this Define touched is already known current.
		o.recordOperationsEvidence(t.id)
	}
	return nil
}

// hasPostStateToVerify reports whether the state after this Task's
// Define is one its postcondition can judge. It is not when Define
// resolved the Task itself without committing a change (Kept, Skipped,
// Block: it chose not to converge, so there is no change to verify,
// E-110, E-112), nor when a dry run
// or preview skipped a mutation Define planned (the observed state is
// the state before the plan, E-097). A planned run whose Define planned
// nothing left the real post-state and is checked as a real run is
// (E-106).
func (t *Task) hasPostStateToVerify() bool {
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	// Block resolves at once, so its terminal Task is never re-checked. A
	// Kept or Skipped inside Define is held as an unratified proposal
	// until the callback's return confirms it; it claims "no change" only
	// while Define committed no Effect (E-112): a Kept after a real
	// mutation still owes its postcondition.
	blocked := st != nil && record.IsTerminalTask(st.node.Rec.State())
	keptUnchanged := st != nil && st.node.HasProposal() && !o.hasRecordedEffectLocked(t.id)
	o.mu.Unlock()
	if blocked || keptUnchanged {
		return false
	}
	return !o.cfg.dryRun || !o.hasPlannedEffect(t.id)
}

// recordOperationsEvidence records the after-Define Evidence phase
// from tracked operations (§9.2/§30) when taskID recorded at least one —
// a Task with no Verify and no tracked operation leaves Evidence
// unevaluated rather than manufacturing a claim (§9.2's closing rule).
func (o *Run) recordOperationsEvidence(taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil || o.taskFreshness.OperationCount(st.TaskID()) == 0 {
		return
	}
	st.node.Rec.RecordAfterEvidence(evidencePhase{Evaluated: true, Satisfied: true, Source: operationsEvidenceSource})
	o.emitWireEventLocked(wire.EventEvidenceEvaluated, taskID, map[string]any{
		"phase":     "after_definition",
		"evaluated": true,
		"satisfied": true,
		"source":    operationsEvidenceSource,
	})
}

// passthroughCallbackOutcome hands a Define callback's (or a Verify
// observation's) own error back to the scheduler's generic resolution path
// (executeWork/resolveObserved) completely unchanged — that path renders
// err.Error() verbatim as the Task's Fail summary, so wrapping it here would
// prepend internal plumbing text ("runDefine: ...") onto what the reader
// sees, the same reason Fail/Block keep the caller's own wording intact.
// The task itself may already be resolved by the time this runs (a
// pre/post-Verify failure calls failScheduled/failScheduledWithCode before
// returning); this is only ever err's carrier back to executeWork's
// already-terminal branch (see Task.finish).
func passthroughCallbackOutcome(err error) error {
	return err
}

// evaluateVerifiers runs taskID's verifiers (see
// freshness.EvaluateVerifiers) and emits each one's outcome as
// verification.observed (spec §38), named by its registration order since
// Verify registers anonymous funcs.
func evaluateVerifiers(ctx context.Context, o *Run, taskID string, verifiers []freshness.Verifier) (allSatisfied bool, err error) {
	return freshness.EvaluateVerifiers(ctx, verifiers, func(index int, outcome freshness.VerifierOutcome) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.emitWireEventLocked(wire.EventVerificationObserved, taskID, wire.VerificationDoc{
			Name:   fmt.Sprintf("verify_%d", index),
			Status: verificationStatus(outcome),
		}.EventPayload())
	})
}

// verificationStatus maps one verifier's outcome to the §36 verification
// status vocabulary (wire.VerificationSatisfied/Unsatisfied/Error).
func verificationStatus(outcome freshness.VerifierOutcome) string {
	switch outcome {
	case freshness.VerifierErrored:
		return wire.VerificationError
	case freshness.VerifierSatisfied:
		return wire.VerificationSatisfied
	default:
		return wire.VerificationUnsatisfied
	}
}

// evidencePhaseName selects which of a Task's two Verify observation phases
// (§30) a call updates.
type evidencePhaseName int

const (
	evidencePhaseBefore evidencePhaseName = iota
	evidencePhaseAfter
)

// recordEvidencePhase stores one Verify observation phase (§30). satisfied
// is meaningful only when evaluated is true, matching EvidencePhase's own
// documented contract.
func (o *Run) recordEvidencePhase(taskID string, phase evidencePhaseName, evaluated, satisfied bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil {
		return
	}
	recorded := evidencePhase{Evaluated: evaluated}
	if evaluated {
		recorded.Satisfied = satisfied
		recorded.Source = verifyEvidenceSource
	}
	switch phase {
	case evidencePhaseBefore:
		st.node.Rec.RecordBeforeEvidence(recorded)
	case evidencePhaseAfter:
		st.node.Rec.RecordAfterEvidence(recorded)
	}
	o.emitWireEventLocked(wire.EventEvidenceEvaluated, taskID, map[string]any{
		"phase":     evidencePhaseWireName(phase),
		"evaluated": evaluated,
		"satisfied": satisfied,
		"source":    recorded.Source,
	})
}

// evidencePhaseWireName maps an evidencePhaseName to the §38 payload's
// "phase" literal (spec §38 example: "phase": "after_definition").
func evidencePhaseWireName(phase evidencePhaseName) string {
	if phase == evidencePhaseAfter {
		return "after_definition"
	}
	return "before_definition"
}

// markDefineRan records that a Task's Define callback ran and returned
// successfully (§29/§30).
func (o *Run) markDefineRan(taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskStates[taskID]; st != nil {
		st.node.Rec.MarkDefineRan()
		st.markFiling()
	}
}

// failScheduledWithCode is failScheduled with a stable Problem.Code
// attached — the scheduler's own resolution authority (see
// Task.resolveScheduled), used for outcomes that need a
// machine-matchable code rather than only a rendered summary.
func (t *Task) failScheduledWithCode(code, summary string) {
	p := record.SanitizeProblem(problem{Code: code, Summary: txt.Text(summary)})
	t.resolveScheduled(failed, txt.Text(summary), []problem{p})
}

type State struct {
	Dir   string
	AppID string
}

type Result struct {
	Conclusion Conclusion
	Err        error
}

func (r Result) ExitCode() int { return r.Conclusion.exitCode }

type Conclusion struct {
	Outcome     Outcome
	Changed     bool
	DryRun      bool
	Explanation string
	subject     string
	partial     bool
	warned      bool
	cancelled   bool
	tasks       []taskSnapshot
	collections []tasksSnapshot
	changes     []changesSnapshot
	plans       []planSnapshot
	actions     []Action
	exitCode    int
	runID       string
	startedAt   time.Time
	finishedAt  time.Time
	facts       []factRecord
}

func asEvoAction(a record.Action) Action {
	return Action{
		Command: a.Command, Label: a.Label, uRL: a.URL, file: a.File,
		explanation: a.Explanation, requiresConfirmation: a.RequiresConfirmation,
		destructive: a.Destructive,
	}
}

func asRecordAction(a Action) record.Action {
	return record.Action{
		Command: a.Command, Label: a.Label, URL: a.uRL, File: a.file,
		Explanation: a.explanation, RequiresConfirmation: a.requiresConfirmation,
		Destructive: a.destructive,
	}
}

func asEvoActions(in []record.Action) []Action {
	if in == nil {
		return nil
	}
	out := make([]Action, len(in))
	for i, a := range in {
		out[i] = asEvoAction(a)
	}
	return out
}

func asRecordActions(in []Action) []record.Action {
	if in == nil {
		return nil
	}
	out := make([]record.Action, len(in))
	for i, a := range in {
		out[i] = asRecordAction(a)
	}
	return out
}

func asEvoConclusion(c record.Conclusion) Conclusion {
	return Conclusion{
		Outcome: Outcome(c.Outcome), Changed: c.Changed, DryRun: c.DryRun,
		Explanation: c.Explanation, subject: c.Subject,
		partial: c.Partial, warned: c.Warned, cancelled: c.Cancelled,
		tasks: c.Tasks, collections: c.Collections, changes: c.Changes,
		plans: c.Plans, actions: asEvoActions(c.Actions), exitCode: c.ExitCode,
		runID: c.RunID, startedAt: c.StartedAt, finishedAt: c.FinishedAt,
		facts: c.Facts,
	}
}

func asRecordConclusion(c Conclusion) record.Conclusion {
	return record.Conclusion{
		Outcome: record.Outcome(c.Outcome), Subject: c.subject, Changed: c.Changed, Partial: c.partial,
		Warned: c.warned, Cancelled: c.cancelled, Explanation: c.Explanation,
		Tasks: c.tasks, Collections: c.collections, Changes: c.changes,
		Plans: c.plans, Actions: asRecordActions(c.actions), ExitCode: c.exitCode,
		RunID: c.runID, StartedAt: c.startedAt, FinishedAt: c.finishedAt,
		DryRun: c.DryRun, Facts: c.facts,
	}
}

func asRecordResult(r Result) record.Result {
	return record.Result{Conclusion: asRecordConclusion(r.Conclusion), Err: r.Err}
}

type Action struct {
	Command              *commandSpec
	Label                string
	uRL                  string
	file                 string
	explanation          string
	requiresConfirmation bool
	destructive          bool
}
