package evo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// Main runs run under SIGINT and SIGTERM and returns the process exit code;
// it never exits. cfg is optional: only the first Config is used, and any
// further Config is ignored.
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
		out.end(runErr)
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

	// Debug is the lowest slog.Level the debug journal keeps. Zero is
	// slog.LevelInfo, which EVO_DEBUG may lower or raise.
	Debug slog.Level
	// debug configures the debug journal presentation; its level, when
	// unset, comes from Debug.
	debug debugConfig

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
	fileFS fs.FS
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
		debug: debugConfig{
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
	if c.debug.level == levelUnset {
		c.debug.level = logLevelOf(c.Debug)
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
		c.fileFS = fs.Disk{}
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
	opts = append(opts, debugOptions(c.debug)...)
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

// Init is the sole Output constructor. It builds an Output from cfg,
// installs it as the package-level default, and arms first paint — call
// once, in main, before any I/O:
//
//	func main() {
//	    (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: "repo-retire"}, func(ctx context.Context, r *evo.Run) error { return nil })
//	    os.Exit(evo.Main(run))
//	}
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

// finishInit applies the first-paint invariant and records Config facts.
func finishInit(out *Run, cfg Config, facts []factRecord) *Run {
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
	if c.debug.level == levelUnset && c.Debug == 0 {
		if lvl, ok := parseDebugEnv(lookupEnv(envKeyDebug)); ok {
			c.debug.level = lvl
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
	for o.graph.CancelPendingGate(cancelCauseClose) {
	}
	needFinish := !o.finished
	o.mu.Unlock()
	if needFinish {
		_ = o.finish()
	}
	o.mu.Lock()
	o.live.Stop()
	o.stopPlainHeartbeatsLocked()
	o.closed = true
	o.graph.Close()
	o.mu.Unlock()
	o.graph.Cancel()
	// Finish already wrote the manifest and warned when it could not; this
	// releases the Run's exclusive manifest lock (spec §11.3) and returns
	// any write or release failure. Already committed Task records on disk
	// are unaffected — Close never rolls anything back.
	return o.freshness.Session().Close()
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

// explainCancellationLocked names what interrupted a cancelled run as its
// Explanation, ahead of the cancelled Task's own ending. Any other outcome
// keeps its own Explanation untouched.
func (o *Run) explainCancellationLocked(c *record.Conclusion) {
	if c.Outcome != record.OutcomeCancelled {
		return
	}
	if cause := o.graph.CancelCause(); cause != "" {
		c.Explanation = cause
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
)

const (
	exitOK        = record.ExitOK
	exitRefused   = record.ExitRefused
	exitFailed    = record.ExitFailed
	exitCancelled = record.ExitCancelled
)

type evidencePhase = record.EvidencePhase

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
	fileFS fs.FS
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
func withFileFS(fsys fs.FS) option {
	return optionFunc(func(c *config) { c.fileFS = fsys })
}

// Test helpers reachable through the evo type alias (root export_test.go
// cannot attach methods to engine types).

// --- folded from capability.go ---

// runHost is the Run as graph, freshness and change borrow it: its lock,
// its settings, its live rows and its edge to the reader. It implements
// graph.Host, freshness.Narration and change.Engine; the packages make every
// record write they owe themselves.
type runHost struct{ out *Run }

// resourceWaitingPrefix opens the live activity a Task shows while one of its
// claims waits on a conflicting holder.
const resourceWaitingPrefix = "waiting for "

// newChangeHost is the Host this Run's changes report through.
func (o *Run) newChangeHost() change.Host {
	return change.NewHost(change.HostParts{Engine: runHost{out: o}, Record: o.rec, Graph: o.graph, Freshness: o.freshness})
}

func (e runHost) Lock()   { e.out.mu.Lock() }
func (e runHost) Unlock() { e.out.mu.Unlock() }

// DryRun reports whether the Run plans its changes instead of making them.
func (e runHost) DryRun() bool {
	e.out.mu.Lock()
	defer e.out.mu.Unlock()
	return e.out.cfg.dryRun
}

// FS is the filesystem File performs I/O through.
func (e runHost) FS() fs.FS {
	e.out.mu.Lock()
	defer e.out.mu.Unlock()
	return e.out.cfg.fileFS
}

// LedgerTense is the tense id's rows take: the shared guard (open, not yet
// resolved) behind every change a Task records. A Task the Run does not
// know fails without misuse.
func (e runHost) LedgerTense(id record.TaskID) (record.LedgerTense, error) {
	o := e.out
	st := o.taskStates[string(id)]
	if st == nil {
		return 0, errClosed
	}
	if err := e.CheckOpen(); err != nil {
		return 0, err
	}
	if record.IsTerminalTask(st.node.Rec.State()) {
		o.recordMisuseFor(st.name, errAlreadyResolved)
		return 0, errAlreadyResolved
	}
	return record.TenseFor(o.cfg.dryRun), nil
}

// CheckOpen refuses a write once the Run is closing.
func (e runHost) CheckOpen() error {
	err := e.out.ensureOpen()
	if err != nil {
		e.out.recordMisuse(err)
	}
	return err
}

// RecordMisuse fails the Run with err.
func (e runHost) RecordMisuse(err error) { e.out.recordMisuse(err) }

// NoteManifestWarning surfaces a manifest's safe cache-miss warning
// (corrupt or unknown prior manifest, spec §11.3) as a run-scoped Fact,
// never as an error: a miss is always safe to continue past.
func (e runHost) NoteManifestWarning(warning error) { e.out.Fact("manifest", warning.Error()) }

// ShowWaiting makes id's row read "waiting for <label>" and returns the
// function that restores what it showed before.
func (e runHost) ShowWaiting(id record.TaskID, label string) (restore func()) {
	o, taskID := e.out, string(id)
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

// SectionOpened journals a ledger section a change opened.
func (e runHost) SectionOpened(_ record.TaskID, section string, tense record.LedgerTense) {
	e.out.bumpLocked()
	e.out.appendEventLocked(recEvent{Type: tense.DeclaredEvent(), EntityID: section})
}

// EntryRecorded journals a ledger row a change recorded and narrates it as
// the §38 effect line: planned in a planned run, committed otherwise.
func (e runHost) EntryRecorded(task record.TaskID, row change.LedgerRow) {
	o := e.out
	if row.Added {
		o.bumpLocked()
		o.appendEventLocked(recEvent{Type: row.Tense.RecordedEvent(), EntityID: row.Section})
	}
	event := wire.EventEffectPlanned
	if row.Tense == record.TenseChanged {
		event = wire.EventEffectCommitted
	}
	o.emitWireEventLocked(event, string(task), row.Entry.Payload(row.Verb))
}

// VerificationRecorded files the Task's row again and narrates each detail
// as the sanitized copy evo.run projects, so JSONL and JSON agree.
func (e runHost) VerificationRecorded(task record.TaskID, details []record.VerificationDetail) {
	o := e.out
	o.board.Changed(task)
	for _, d := range details {
		o.emitWireEventLocked(wire.EventVerificationObserved, string(task), wire.ToVerificationDoc(d).EventPayload())
	}
}

// stateOf is the render state of the graph's Task t. Callers hold o.mu.
func (o *Run) stateOf(t *graph.Task) *taskState { return o.taskStates[t.ID] }

// Ending is the state and Problem a callback's returned error settles.
func (runHost) Ending(err error) (record.State, record.Problem) { return endingOf(err) }

// Changed follows the record once t was written outside a resolution.
func (e runHost) Changed(*graph.Task) { e.out.followRecordLocked() }

// EndingName names the Task that carries the run's own ending: the
// configured subject, else the executable's name.
func (e runHost) EndingName() string { return e.out.endingName() }

// TaskDeclared registers the row of a Task the graph just declared: its
// render state, its place in its collection's live index and census, the
// declaration events, and a forced paint.
func (e runHost) TaskDeclared(t *graph.Task) {
	o := e.out
	st := &taskState{
		id:     t.ID,
		node:   t,
		name:   t.Name,
		board:  o.board,
		timing: record.TaskTiming{Declared: o.cfg.clock.Now()},
	}
	st.handle = &Task{out: o, id: st.id}
	o.taskStates[st.id] = st
	o.appendTaskLocked(st)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.declared", EntityID: st.id})
	o.emitWireEventLocked(wire.EventTaskDeclared, st.id, map[string]any{"name": t.Name})
	o.live.Signal(true)
}

// GateDeclared registers the gate of a container builder: no row, in no
// collection, but followed like any Task.
func (e runHost) GateDeclared(t *graph.Task) {
	o := e.out
	gate := &taskState{id: t.ID, node: t, name: t.Name, board: o.board}
	gate.handle = &Task{out: o, id: gate.id}
	o.taskStates[gate.id] = gate
}

// ContainerDeclared registers a Group or Sequence the graph just declared.
func (e runHost) ContainerDeclared(c *graph.Container) {
	o := e.out
	st := &tasksState{id: c.ID, out: o, node: c, name: c.Name}
	parentID := ""
	if c.Parent == nil {
		o.collections = append(o.collections, st)
	} else {
		parentID = c.Parent.ID
	}
	o.containerStates[st.id] = st
	o.board.DeclareGroup(st.id, st.name, parentID)
	st.handle = &Group{out: o, id: st.id}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, parentID)
}

// Started is the render work a claimed Task owes: the eligibility event, the
// move to Running, the snapshot version and a forced paint, within the live
// render budget, since a start is the spinner FP-005 requires before the
// check. It reports whether the Task began: false when an interrupt settled
// it between its claim and this step, so the scheduler skips its callback.
func (e runHost) Started(t *graph.Task) bool {
	if t.IsGate() {
		return true
	}
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
	st := o.stateOf(t)
	if st == nil || record.IsTerminalTask(t.Rec.State()) {
		// An interrupt settled the Task between its claim and this step: it
		// never started, so it is neither eligible nor started.
		return false
	}
	o.emitWireEventLocked(wire.EventTaskEligible, st.id, nil)
	if t.Rec.State() == pending {
		o.promoteRunningLocked(st)
	} else {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.live.Signal(true)
	return true
}

// Resolving lets a running row finish its spinner frame before it resolves.
func (e runHost) Resolving(t *graph.Task) { e.out.live.HoldRunning(record.TaskID(t.ID)) }

// AlreadyResolved records a second terminal verb on t as misuse, keeping
// the rejected call's own summary so the misuse line can show what got
// dropped (release-gate round 5 finding 4).
func (e runHost) AlreadyResolved(t *graph.Task, summary string) {
	e.out.recordAlreadyResolvedLocked(t.Name, summary)
}

// Disowned disowns every Effect in flight for t: an Effect whose callback
// resolved its own Task as anything but Done and then returned nil handled
// the failure itself, and its work must not reach the ledger.
func (e runHost) Disowned(t *graph.Task) {
	if id := t.TaskID(); e.out.changes.EffectInFlight(id) {
		e.out.changes.DenyEffect(id)
	}
}

// Settled follows the record and writes the settled Task where its readers
// find it. A Group or Sequence child repaints the live ledger; a standalone
// Task commits its row and its own File/Exec ledger rows to scrollback at
// once, so later Printf/Confirm output can never land above finished work.
// A Done Task commits its manifest record; Failed, Blocked, and Cancelled
// never do (spec §8.2/§11.3).
func (e runHost) Settled(t *graph.Task) {
	o := e.out
	o.followRecordLocked()
	st := o.stateOf(t)
	if st == nil {
		return
	}
	if st.collection() != nil {
		o.live.Signal(true)
	} else {
		o.commitResolvedTaskLocked(st.id)
		o.commitNamedEffectsLocked(st.id)
	}
	if t.Rec.State().Succeeded() {
		o.commitManifestLocked(st)
	}
}

// commitManifestLocked persists st's accumulated operations as this Run's
// truth (spec §11.3) once it settled Done, and narrates the commit; see
// freshness.Run.Commit. Callers hold o.mu.
func (o *Run) commitManifestLocked(st *taskState) {
	commit := o.freshness.Commit(o.graph.Context(), st.node.TaskID(), freshness.ModeFor(o.cfg.dryRun))
	if commit.Err != nil {
		o.warnManifestUnsavedLocked(commit.Err)
		return
	}
	if commit.Written {
		o.emitWireEventLocked(wire.EventManifestTaskCommitted, st.id, map[string]any{
			"operations": commit.Operations,
		})
	}
}

// RunEnded registers the synthetic Task that carries the run's own ending:
// one the caller never declared, already settled.
func (e runHost) RunEnded(t *graph.Task) {
	o := e.out
	o.appendTaskLocked(&taskState{id: t.ID, node: t, name: t.Name, board: o.board, synthetic: true})
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "output." + string(t.Rec.State())})
}

// DefinitionStarted narrates that t's callback was entered and files its
// row again.
func (e runHost) DefinitionStarted(t *graph.Task) {
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventDefinitionStarted, t.ID, nil)
	o.board.Changed(record.TaskID(t.ID))
}

// DefinitionFinished narrates that t's callback returned.
func (e runHost) DefinitionFinished(t *graph.Task, failed bool) {
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventDefinitionFinished, t.ID, map[string]any{"failed": failed})
}

// EvidenceEvaluated narrates one evidence phase (§30) as evidence.evaluated.
func (e runHost) EvidenceEvaluated(t *graph.Task, phase graph.EvidencePhase, evidence record.EvidencePhase) {
	name := "before_definition"
	if phase == graph.EvidenceAfter {
		name = "after_definition"
	}
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventEvidenceEvaluated, t.ID, map[string]any{
		"phase":     name,
		"evaluated": evidence.Evaluated,
		"satisfied": evidence.Satisfied,
		"source":    evidence.Source,
	})
}

// BasisJudged narrates the verdict on a Task's Basis as basis.fingerprinted.
func (e runHost) BasisJudged(id record.TaskID, judgement freshness.BasisJudgement) {
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventBasisFingerprinted, string(id), map[string]any{
		"count": len(judgement.Observed), "current": judgement.Current,
	})
}

// VerifierObserved narrates one Verify check's outcome as
// verification.observed (spec §38), named by its registration order since
// Verify registers anonymous funcs.
func (e runHost) VerifierObserved(id record.TaskID, index int, outcome freshness.VerifierOutcome) {
	status := wire.VerificationUnsatisfied
	switch outcome {
	case freshness.VerifierErrored:
		status = wire.VerificationError
	case freshness.VerifierSatisfied:
		status = wire.VerificationSatisfied
	}
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventVerificationObserved, string(id), wire.VerificationDoc{
		Name:   fmt.Sprintf("verify_%d", index),
		Status: status,
	}.EventPayload())
}

// Observe narrates one step of a tracked operation as the §38 "evo.event"
// line it has always been: change speaks of subjects and steps, the wire
// vocabulary stays at this edge.
func (e runHost) Observe(ob change.Observation) {
	e.out.emitWireEventLocked(observationEvent(ob.Kind), string(ob.Task), observationPayload(ob))
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

// --- folded from misuse.go ---

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
	o.graph.Root().Log(txt.MisuseLine(o.misuse.Hint(), !o.cfg.noColor))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Fail/Block/Warn/Cancel/Skipped) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Run) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	o.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}

// --- folded from redact.go ---

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

// Fact accumulates a run-scoped discovered name/value annotation (P8
// symmetry with Task.Fact) — information about the run, fire-and-
// forget: it both renders a durable dim "name  value" line immediately
// (the same "act now" contract Println has) and stores the annotation for
// the structured Snapshot/JSON views. A nil Output is safe and records
// nothing.
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
	r.writeDurableTextLocked(txt.Dim(f.Name+"  "+f.Value, !r.cfg.noColor) + "\n")
	return r
}

// warnLocked records p as a run-scoped warning and renders it. Callers
// must already hold o.mu. Public Warn was removed in 1.1; this is the
// engine's own path (a failed manifest flush) onto the same projection
// Task.Problem(..., Severity(SeverityWarning)) uses for the
// conclusion's warned modifier.
func (o *Run) warnLocked(p problem) {
	o.graph.Root().Warn(p)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "run.warned", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(o.cfg.glyphs), txt.SGRYellow, !o.cfg.noColor)
	o.writeDurableTextLocked(glyph + " " + p.Summary + "\n")
}

// --- folded from run_signal.go ---

// interrupt stops the run at the first signal. The graph owns the order that
// leaves the ledger honest (see graph.Graph.Interrupt) and what it cancels
// (see graph.Graph.CancelActive).
func (o *Run) interrupt(reason string) {
	if o == nil {
		return
	}
	o.graph.Interrupt(cancelCauseUser, func() { o.graph.CancelActive(reason) })
}

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

// Verify registers an advanced current-state observation check (§9.1),
// ANDed with any previously registered check on this Task. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define — a call after Define is ignored and recorded as misuse.
//
// Define's own execution wiring (see graph.Define) runs every registered
// Verify twice: once before the callback (with the Task already Running) —
// all true skips the callback entirely and settles the Task Satisfied;
// any false enters the callback; an
// observation error fails the Task without entering the callback — and
// once after a successful callback, where any false fails the Task with
// ProblemCodeVerificationUnsatisfied and an observation error fails it
// plainly. Neither check commits a success record on its own; only a fully
// satisfied pass (pre- or post-) does. The after-check is skipped in two
// cases only: Define resolved the Task itself (Block, or Kept/Skipped with
// no Effect committed first), or a dry run or preview skipped an Effect
// Define planned, so the observed state is the one before the plan. A
// planned run whose Define planned nothing is checked like a real one.
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
	o.freshness.Tasks().AddVerifier(st.node.TaskID(), freshness.Verifier(fn))
	return t
}

// Define freezes this Task's dependency/verification configuration and
// submits fn to the scheduler (§7): Define itself returns immediately after
// submission, before fn ever runs; the scheduler starts fn as soon as the
// Task is eligible; fn's error becomes the Task's outcome (see graph.Define
// for the Verify-aware wiring); evo.Run/evo.Main wait for every submitted
// Task before returning their Result.
//
// Define returns the same *Task (ZYS-849 Decisions) so the common
// single-Task shape can be written `return task.Define(fn).Wait()`. This is
// fluent sugar only — it does not change Define's asynchronous submission:
// fn still runs on the scheduler, not inline before Define returns.
//
// A second Define on the same Task is misuse — configuration freezes once.
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
	o.mu.Unlock()
	if st == nil {
		o.recordMisuse(t.rejected)
		return t
	}
	var cfg defineConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	graph.Define(o.graph, st.node, o.definition(st, o.cleanStops.wrap(cfg.cleanStopSignals, fn)))
	return t
}

// definition is st's Define with fn as its callback: the operations inside
// it act through the Run's changes, and freshness checks it.
func (o *Run) definition(st *taskState, fn func(context.Context) error) graph.Definition[*change.Run] {
	return graph.Definition[*change.Run]{Owner: o.changes, Checks: o.freshness.Checks(st.node.TaskID()), Callback: fn}
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
		Command: argvOf(a.Command), Label: a.Label, uRL: a.URL, file: a.File,
		explanation: a.Explanation, requiresConfirmation: a.RequiresConfirmation,
		destructive: a.Destructive,
	}
}

func asRecordAction(a Action) record.Action {
	return record.Action{
		Command: commandOf(a.Command), Label: a.Label, URL: a.uRL, File: a.file,
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

// Action is a recommended next step: argv a machine can run, or text only a
// human can act on.
type Action struct {
	Command              []string
	Label                string
	uRL                  string
	file                 string
	explanation          string
	requiresConfirmation bool
	destructive          bool
}

// argvOf is cmd as argv: the executable, then its arguments.
func argvOf(cmd *commandSpec) []string {
	if cmd == nil {
		return nil
	}
	return append([]string{cmd.Executable}, cmd.Args...)
}

// commandOf is argv as the record's command, or nil for an empty argv.
func commandOf(argv []string) *commandSpec {
	if len(argv) == 0 {
		return nil
	}
	return &commandSpec{Executable: argv[0], Args: slices.Clone(argv[1:])}
}
