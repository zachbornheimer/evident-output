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
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
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
	sigCh := make(chan process.Signal, signalChannelCapacity)
	notifySignals(sigCh, process.Interrupt, process.Terminate)
	defer stopSignals(sigCh)

	// runCtx becomes o.Context() for the duration of this run (see
	// beginRunContext): every Define/Verify task scope started from here
	// on descends from the caller's own ctx, not just from run's local
	// parameter.
	runCtx := out.beginRunContext(ctx)

	done := make(chan error, 1)
	go func() {
		// Delivered from a defer so a callback that ends its goroutine via
		// runtime.Goexit (directly, or through a library such as
		// testing.T.FailNow) still releases the waiter below.
		var (
			err      error
			returned bool
		)
		defer func() {
			if !returned {
				err = errRunExitedWithoutReturn
			}
			done <- err
		}()
		if run != nil {
			err = run(runCtx)
		}
		returned = true
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
		if t.node.Rec.State() == Blocked {
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
		if t.node.Rec.State() == Failed {
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
type ColorMode int

const (
	// ColorAuto uses TTY detection and honors NO_COLOR.
	ColorAuto ColorMode = iota
	// ColorAlways forces semantic color even off a TTY.
	ColorAlways
	// ColorNever disables color.
	ColorNever
)

// Format selects the overall projection mode. Zero is ordinary human output.
type Format int

const (
	// FormatHuman is ordinary human (and optional interactive) presentation.
	FormatHuman Format = iota
	// FormatData reserves stdout for the application's domain payload; Evo
	// human presentation goes to stderr (data-command mode).
	FormatData
	// FormatExternal disables inline rendering (snapshots only).
	FormatExternal
	// FormatJSON writes one final v2 "evo.run" document to Stdout at Finish;
	// human presentation still goes to Stderr, exactly as FormatData routes
	// it (spec §32.1). Do not infer this from stdout being a pipe — it is
	// always an explicit top-level choice.
	FormatJSON
	// FormatJSONL streams v2 "evo.event" JSON lines to Stdout as they occur,
	// plus a final run.finished line; human presentation goes to Stderr
	// (spec §32.1).
	FormatJSONL
)

// ParseFormat parses one of "human", "data", "external", "json", "jsonl"
// (case-insensitive, surrounding whitespace ignored) into a Format — the
// public entry point a host CLI's own --format/--json flag binds to (spec
// §32.1). Evo does not parse os.Args itself.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "human":
		return FormatHuman, nil
	case "data":
		return FormatData, nil
	case "external":
		return FormatExternal, nil
	case "json":
		return FormatJSON, nil
	case "jsonl":
		return FormatJSONL, nil
	default:
		return 0, fmt.Errorf("%w: unknown format %q", ErrInvalidConfig, s)
	}
}

// Verbosity selects which message visibilities are projected to the human stream.
// Zero is normal (non-verbose) human detail.
type Verbosity int

const (
	// VerbosityNormal projects Normal-visibility messages only.
	VerbosityNormal Verbosity = iota
	// VerbosityVerbose also projects Verbose-visibility messages. It also
	// expands each TaskHandle.Skipped/Kept taxonomy row from its default
	// aggregated "! skipped N (reason1, reason2)" count into one named line
	// per reason ("reason: name1, name2, ..."). The names themselves are
	// never lost at VerbosityNormal — they are always present on the Go
	// TaskSnapshot.Skipped/Kept fields (returned by Output.Snapshot and
	// TaskHandle.Snapshot); VerbosityVerbose only changes whether the plain
	// human render surfaces them. The wire JSONDocument (JSONTask) does not
	// currently carry Skipped/Kept at all — read the Go snapshot directly to
	// get the names programmatically.
	VerbosityVerbose
)

// DebugConfig configures the debug journal presentation.
type DebugConfig struct {
	// Level is the minimum debug journal level. Zero (LevelUnset) resolves to
	// LevelInfo. Use LevelTrace or LevelDebug to surface Debug/Capture mirrors.
	Level LogLevel
	// View selects history vs pane presentation (default History).
	View DebugPresentation
	// PaneHeight is used when View is DebugPresentationPane (default 5).
	PaneHeight int
	// NewestFirst orders the pane (default true when zero-config pane).
	NewestFirst *bool
	// PreserveAlways forces a diagnostic tail on every Finish in pane mode.
	PreserveAlways bool
	// AddSource resolves each record's call site to a source=file.go:line
	// field on human/pane/history rendering (slog.HandlerOptions.AddSource
	// semantics). Off by default: the raw program counter is always kept on
	// LogRecord.PC for machine consumers, but a human debug line never shows
	// a bare pc=<uintptr> unless this is set.
	AddSource bool
}

// Config is the sole application-facing construction surface.
//
// Zero values mean automatic/default behavior. Use DefaultConfig() when you
// need a mutable baseline for advanced fields.
//
//	out := evo.Init()
//	out := evo.Init(evo.Config{Title: "bpp-csharp"})
//	cfg := evo.DefaultConfig(); cfg.Title = "x"; out := evo.Init(cfg)
type Config struct {
	// Title is the subject shown in the conclusion (formerly For's argument).
	Title string

	// Subject is an optional durable line rendered once, immediately, right
	// under the title — a repo path, a target host, the thing every
	// projection needs the reader to see up front. Set it once in Config
	// instead of calling out.Println(root) (or whatever the identifying
	// value is) at every projection/command that needs to show it.
	//
	// When the subject text isn't known until after Init (e.g. resolved
	// from a flag parsed later, but still before any other I/O), call
	// Output.subject(text) instead — same one-shot durable-line semantics,
	// as a post-construction setter (I3).
	Subject string

	// Facts are run-scoped name/value lines painted with the header, before
	// any Task. Use them for values known at construction (scan roots, the
	// repo path). Discovery that happens during work belongs on the Task
	// that learned it (task.Fact), not here.
	Facts []FactRecord

	// Stdout is the ordinary human stream (default os.Stdout).
	// In FormatData mode, Stdout is reserved for domain payload via ResultWriter;
	// human presentation moves to Stderr.
	Stdout io.Writer
	// Stderr owns diagnostics by default (default os.Stderr).
	Stderr io.Writer
	// Result is an optional domain-payload writer. When nil and Format is
	// FormatData, ResultWriter returns Stdout. Presentation never writes here.
	Result io.Writer
	// Stdin is the facade Confirm reads one answer line from (default os.Stdin).
	Stdin io.Reader

	// Verbosity gates Verbose() print messages (default VerbosityNormal).
	Verbosity Verbosity
	// Color policy (default ColorAuto).
	Color ColorMode
	// Format projection mode (default FormatHuman). Stream routing only —
	// FormatData does not mean JSON. Presentation encoding is Projection.
	Format Format
	// Projection selects presentation encoding (human, plain, json, jsonl,
	// stream-json). Zero is unspecified: Init applies EVO_OUTPUT when set,
	// otherwise human TTY/plain inference. Independent of Format.
	Projection Projection

	// Debug configures the debug journal.
	Debug DebugConfig

	// Advanced (optional) — zero values inherit safe defaults.
	Clock    TimeSource
	Redactor Redactor
	Terminal TerminalDriver
	Strict   bool
	Width    int
	// VisibilityDelay is the wait before the first live paint.
	// nil means default (80ms). Non-nil is exact, including 0 for immediate.
	// Use evo.Delay(d) to set a value from a duration literal.
	VisibilityDelay *time.Duration
	MaxFrameRate    int
	MaxEntities     int
	MaxEvents       int
	// Plain disables live interactive frames, on a TTY or off (C3: replaces
	// the former separate ForcePlain and NonInteractive fields — every read
	// site combined them with OR, so there was never a distinct behavior
	// between the two to preserve).
	Plain bool

	// Glyphs selects the state-glyph vocabulary (default GlyphsAuto: Unicode
	// off a TTY or on a UTF-8 locale, ASCII on a non-UTF-8 interactive TTY).
	Glyphs GlyphProfile

	// FailedExitCode is the process exit code when the conclusion is failed.
	// Zero means use ExitFailed (2). Set to 1 for conventional CLI tools that
	// treat any non-zero failure as exit 1 (e.g. quality gates / git hooks).
	FailedExitCode int

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
	Preview bool

	// Isolated returns an independent Output that is not installed as the
	// package-level default. First paint still arms — Isolated is not a
	// blank-terminal exemption. Use for parallel tests and embedders that
	// hold their own *Output instead of going through Default()/Task().
	// This is the one and only opt-out from default installation.
	Isolated bool

	// MaxConcurrency is the scheduler ceiling: it bounds every executing
	// callback, including work a waiting goroutine runs itself. Zero means
	// GOMAXPROCS.
	MaxConcurrency int

	// StateDir, when non-empty, is the exact manifest state directory File's
	// reconciliation history is read from and committed to
	// (<StateDir>/manifest-v1.json) — bypassing the default
	// os.UserCacheDir()/evident-output/<app-id>/<workspace-hash>/ derivation
	// entirely (spec §11.3). Tests set this to t.TempDir() for isolation.
	StateDir string

	// AppID overrides the derived application id (main-module path plus
	// executable basename) used in the default manifest cache path.
	// Ignored when StateDir is set.
	AppID string

	// ProcessRunner overrides the facade evo.Exec spawns child processes
	// through (spec §8.4). Nil uses the real OS process runner; tests
	// inject testkit's scripted fake for deterministic Exec coverage.
	ProcessRunner ProcessRunner

	// FileFS overrides the facade evo.File performs filesystem I/O through
	// (spec §8.2). Nil uses the real OS filesystem; tests inject a fake to
	// script a reconciliation outcome (e.g. a chmod failure) deterministically.
	FileFS FileFS
}

// Delay returns a non-nil *time.Duration for Config fields where zero is meaningful.
//
//	cfg.VisibilityDelay = evo.Delay(0)                      // immediate
//	cfg.VisibilityDelay = evo.Delay(80 * time.Millisecond) // explicit default
func Delay(d time.Duration) *time.Duration {
	return &d
}

// DefaultConfig returns a fresh ordinary CLI configuration.
// Mutating the result does not affect later DefaultConfig() calls.
func DefaultConfig() Config {
	return Config{
		// Stdout/Stderr filled at resolve time so tests can still pass buffers
		// without forcing os.Stdout into DefaultConfig equality checks.
		Debug: DebugConfig{
			Level: LevelInfo,
			View:  DebugPresentationHistory,
		},
		Width:           defaultWidth,
		VisibilityDelay: Delay(defaultVisibilityDelay),
		MaxFrameRate:    defaultMaxFrameRate,
		MaxEntities:     defaultMaxEntities,
		MaxEvents:       defaultMaxEvents,
	}
}

// resolveConfig fills zero-value fields with ordinary CLI defaults.
func resolveConfig(c Config) Config {
	base := DefaultConfig()
	if c.Stdout == nil {
		c.Stdout = terminal.Stdout()
	}
	if c.Stderr == nil {
		c.Stderr = terminal.Stderr()
	}
	if c.Stdin == nil {
		c.Stdin = terminal.Stdin()
	}
	// LevelUnset (zero) → LevelInfo. LevelTrace is non-zero and selectable via Config.
	if c.Debug.Level == LevelUnset {
		c.Debug.Level = LevelInfo
	}
	if c.Width <= 0 {
		c.Width = base.Width
	}
	// nil = unspecified → default; non-nil (including 0) is intentional.
	if c.VisibilityDelay == nil {
		c.VisibilityDelay = Delay(defaultVisibilityDelay)
	}
	if c.MaxFrameRate <= 0 {
		c.MaxFrameRate = base.MaxFrameRate
	}
	if c.MaxEntities <= 0 {
		c.MaxEntities = base.MaxEntities
	}
	if c.MaxEvents <= 0 {
		c.MaxEvents = base.MaxEvents
	}
	if c.Clock == nil {
		c.Clock = systemClock{}
	}
	if c.Redactor == nil {
		c.Redactor = process.NoopRedactor{}
	}
	if c.ProcessRunner == nil {
		c.ProcessRunner = process.System()
	}
	if c.FileFS == nil {
		c.FileFS = osFileFS{}
	}
	return c
}

func newFromConfig(c Config) *Output {
	opts := configToOptions(c)
	return newOutput(c.Title, opts...)
}

// withVerbosity stores verbosity on the internal config.
func withVerbosity(v Verbosity) Option {
	return optionFunc(func(c *config) { c.verbosity = v })
}

// withFailedExitCode stores a non-default failed conclusion exit code.
func withFailedExitCode(code int) Option {
	return optionFunc(func(c *config) { c.failedExitCode = code })
}

// Title sets the conclusion subject on the internal option config.
func Title(subject string) Option {
	return optionFunc(func(c *config) { c.subject = subject })
}

// --- folded from construct_options.go ---

// configToOptions translates Config into the internal Option list, one
// concern at a time. Options apply in order, so a later concern may refine
// an earlier one (plain() after a live terminal, for example).
func configToOptions(c Config) []Option {
	var opts []Option
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
	switch c.Format {
	case FormatData, FormatJSON, FormatJSONL:
		return c.Stderr
	default:
		return c.Stdout
	}
}

// streamOptions routes each stream by Format.
func streamOptions(c Config) []Option {
	switch c.Format {
	case FormatData:
		// Human presentation on stderr; domain payload on Result (default Stdout).
		resultW := c.Result
		if resultW == nil {
			resultW = c.Stdout
		}
		return []Option{to(c.Stderr), withDiagnostics(c.Stderr), dataProjection(), resultStream(resultW)}
	case FormatExternal:
		opts := []Option{to(c.Stdout), withDiagnostics(c.Stderr), externalProjection()}
		if c.Result != nil {
			opts = append(opts, resultStream(c.Result))
		}
		return opts
	case FormatJSON, FormatJSONL:
		// Human presentation on stderr, exactly like FormatData; the v2
		// wire document/event stream is Evo's own write (not the app's
		// domain payload), so it uses wireStream/wireFormat rather than
		// Result/resultStream (spec §32.1: "stdout: exactly one final JSON
		// document"/"stdout: JSON event lines only" — never c.Result, which
		// stays the FormatData-only escape hatch).
		return []Option{to(c.Stderr), withDiagnostics(c.Stderr), wireFormatOption(c.Format), wireStreamOption(c.Stdout)}
	default:
		opts := []Option{to(c.Stdout), withDiagnostics(c.Stderr)}
		if c.Result != nil {
			opts = append(opts, resultStream(c.Result))
		}
		return opts
	}
}

// colorOptions turns color off for ColorNever, and for ColorAuto when
// NO_COLOR is set or the human writer is not a terminal.
func colorOptions(c Config) []Option {
	noColor := false
	switch c.Color {
	case ColorNever:
		noColor = true
	case ColorAlways:
	default: // ColorAuto
		noColor = lookupEnv(envKeyNoColor) != "" || !writerIsCharDevice(humanWriter(c))
	}
	if noColor {
		return []Option{withNoColor()}
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
func terminalOptions(c Config) []Option {
	var opts []Option
	liveWriter := humanWriter(c)
	wantLive := !c.Plain && rendersInline(c.Format) && writerIsCharDevice(liveWriter)
	switch {
	case c.Terminal != nil:
		opts = callerTerminalOptions(c)
	case wantLive:
		opts = liveTerminalOptions(c, liveWriter)
	default:
		opts = []Option{plain()}
	}
	if c.Plain || c.Projection.forcesPlain() {
		opts = append(opts, plain())
	}
	return append(opts, withProjection(c.Projection))
}

// rendersInline reports whether f presents inline at all; FormatExternal
// is snapshots only.
func rendersInline(f Format) bool {
	switch f {
	case FormatHuman, FormatData, FormatJSON, FormatJSONL:
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
func callerTerminalOptions(c Config) []Option {
	opts := []Option{withTerminal(c.Terminal)}
	sr, ok := c.Terminal.(sinkReporter)
	if !ok {
		return opts
	}
	if sink := sr.Sink(); sink != nil {
		switch sink {
		case c.Stdout, c.Stderr:
			opts = append(opts, withPrimarySharesTerminal())
		default:
			opts = append(opts, to(sink))
		}
	}
	if sameTerminalDevice(sr.Sink(), c.Stderr) {
		opts = append(opts, withDiagnosticSharesTerminal())
	}
	return opts
}

// liveTerminalHeight is the live region's height when the terminal cannot
// report its own.
const liveTerminalHeight = 24

// liveTerminalOptions wires the interactive live region onto liveWriter,
// a real TTY.
func liveTerminalOptions(c Config, liveWriter io.Writer) []Option {
	width, height := c.Width, liveTerminalHeight
	if width <= 0 {
		width = defaultWidth
	}
	// Prefer real terminal dimensions when liveWriter is a TTY *os.File. A
	// TTY without ioctl size (pty, ssh, `timeout`) keeps the default
	// geometry so a spinner still appears.
	if tw, th, ok := terminal.WriterSize(liveWriter); ok {
		// Caller Width>0 is a deterministic override; otherwise use real cols.
		if c.Width <= 0 || c.Width == defaultWidth {
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
	opts := []Option{
		withTerminal(terminal.NewANSI(liveWriter, ansiOpts...)),
		withWidth(width),
		// liveWriter is the same stream to() was already given (c.Stdout,
		// or c.Stderr when machine output owns Stdout) — the terminal and
		// primary are one physical destination, so Finish must not
		// dual-write the conclusion band a second time.
		withPrimarySharesTerminal(),
	}
	// withDiagnostics(c.Stderr) is a distinct io.Writer from liveWriter in
	// the realistic default (to(Stdout), withDiagnostics(Stderr)), but on an
	// interactive shell without redirection both fds name the same
	// controlling tty — detect that here so Debug routes through live-aware
	// sequencing instead of a raw dual-stream write (gate-7 finding 1).
	if sameTerminalDevice(liveWriter, c.Stderr) {
		opts = append(opts, withDiagnosticSharesTerminal())
	}
	return opts
}

// runtimeOptions carries the run's facades and limits.
func runtimeOptions(c Config) []Option {
	var opts []Option
	if c.Stdin != nil {
		opts = append(opts, stdin(c.Stdin))
	}
	visDelay := defaultVisibilityDelay
	if c.VisibilityDelay != nil {
		visDelay = *c.VisibilityDelay
	}
	return append(opts,
		withClock(c.Clock), redact(c.Redactor), withWidth(c.Width),
		visibilityDelay(visDelay), maxFrameRate(c.MaxFrameRate),
		maxEntities(c.MaxEntities), maxEvents(c.MaxEvents),
		maxConcurrency(c.MaxConcurrency),
		withStateDir(c.StateDir), withAppID(c.AppID),
		withProcessRunner(c.ProcessRunner),
		withFileFS(c.FileFS),
	)
}

// debugOptions configures the debug journal and its presentation.
func debugOptions(d DebugConfig) []Option {
	opts := []Option{debugLevel(d.Level)}
	if d.AddSource {
		opts = append(opts, debugAddSource())
	}
	if d.View != DebugPresentationPane {
		return append(opts, debugHistory())
	}
	var paneOpts []DebugPaneOption
	if d.PaneHeight > 0 {
		paneOpts = append(paneOpts, paneHeight(d.PaneHeight))
	}
	if d.NewestFirst != nil {
		if *d.NewestFirst {
			paneOpts = append(paneOpts, newestFirst())
		} else {
			paneOpts = append(paneOpts, oldestFirst())
		}
	}
	if d.PreserveAlways {
		paneOpts = append(paneOpts, preserveDebugTail())
	}
	return append(opts, debugPane(paneOpts...))
}

// modeOptions carries the run's modes: strictness, verbosity, exit code,
// dry run and preview, and glyphs.
func modeOptions(c Config) []Option {
	var opts []Option
	if c.Strict {
		opts = append(opts, strict())
	}
	opts = append(opts, withVerbosity(c.Verbosity))
	if c.FailedExitCode != 0 {
		opts = append(opts, withFailedExitCode(c.FailedExitCode))
	}
	if c.DryRun || c.Preview {
		opts = append(opts, dryRun())
		if c.Subject != "" {
			opts = append(opts, dryRunHeader(c.Subject))
		}
	}
	if c.Preview {
		opts = append(opts, preview())
	}
	return append(opts, Glyphs(c.Glyphs))
}

// --- folded from default.go ---

// defaultMu guards defaultOut, the package-level default instance installed
// by Init/SetDefault and read by Default and the package-level Task/Item/
// Print* facades below (mirrors slog.SetDefault).
var (
	defaultMu  sync.RWMutex
	defaultOut *Output
)

// Init is the sole Output constructor. It builds an Output from cfg,
// installs it as the package-level default, and arms first paint — call
// once, in main, before any I/O:
//
//	func main() {
//	    evo.Init(evo.Config{Title: "repo-retire"})
//	    os.Exit(evo.Main(run))
//	}
//
// evo.Init(evo.Config{}) (or evo.Init(evo.DefaultConfig())) builds an
// ordinary default instance. Config.Isolated returns an independent
// instance that is not installed as the package default. First paint still
// arms — Isolated is not a blank-terminal exemption.
//
// Init fills still-zero Config fields from EVO_OUTPUT, EVO_COLOR,
// EVO_VERBOSE, EVO_DEBUG, and NO_COLOR before TTY inference.
// Explicit Config values win over env; env wins over TTY.
func Init(configs ...Config) *Output {
	cfg := resolveInitConfig(configs)
	cfg = applyEnv(cfg)
	resolved := resolveConfig(cfg)
	out := newFromConfig(resolved)
	return finishInit(out, cfg, resolved.Facts)
}

// finishInit applies the package-default install (unless Isolated) and the
// first-paint invariant. Isolated skips SetDefault only.
func finishInit(out *Output, cfg Config, facts []FactRecord) *Output {
	if !cfg.Isolated {
		SetDefault(out)
	}
	out.changes.Workspace() // captured at Run start: relative paths never follow a later chdir
	out.arm()
	if cfg.Subject != "" && !cfg.DryRun && !cfg.Preview {
		out.Println(cfg.Subject)
	}
	for _, fact := range facts {
		out.Fact(fact.Name, fact.Value)
	}
	return out
}

// resolveInitConfig picks Init's effective Config from its variadic
// argument: zero args is the zero Config (evo.Init()), and one or more
// uses the first.
func resolveInitConfig(configs []Config) Config {
	if len(configs) == 0 {
		return Config{}
	}
	return configs[0]
}

// SetDefault installs out as the package-level default Output.
func SetDefault(out *Output) {
	defaultMu.Lock()
	defaultOut = out
	defaultMu.Unlock()
}

// Default returns the package-level default Output, lazily creating one with
// a zero Config the first time it's needed — package-level Task/Print*
// never panic even when the caller skipped Init.
func Default() *Output {
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
func Task(name string) *TaskHandle {
	return Default().Task(name)
}

// Sequence declares an ordered task container on the default instance —
// see Output.Sequence.
func Sequence(name string) *SequenceHandle {
	return Default().Sequence(name)
}

// Group declares an independent collection of child tasks on the default
// instance — see Output.Group.
func Group(name string) *GroupHandle {
	return Default().Group(name)
}

// Reason returns a get-or-create taxonomy Reason by name on the default
// instance registry — duplicate strings merge into one bucket, so an inline
// evo.Reason("protected") at every call site is always legal; lifting it to a
// package var (var reasonProtected = evo.Reason("protected")) is optional,
// not required for correctness.
func Reason(name string) TaxonomyReason {
	return Default().reasonGetOrCreate(name)
}

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func Print(args ...any) {
	Default().Print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func Printf(format string, args ...any) {
	Default().Printf(format, args...)
}

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func Println(args ...any) {
	Default().Println(args...)
}

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func Verbose() *Printer {
	return Default().at(VisibilityVerbose)
}

// SlogHandler returns a slog.Handler journaling to the default instance —
// package-level sugar (release-gate round 8 finding 6) matching Task/Verbose:
// a caller using the default-instance facade throughout a run should never
// have to reach for a hosted *Output just for the slog bridge. See
// Output.SlogHandler for the level policy and full contract.
func SlogHandler() slog.Handler {
	return Default().slogHandler()
}

// --- folded from env.go ---

// Environment keys honored at Init when the matching Config field is unset.
const (
	envKeyOutput  = "EVO_OUTPUT"
	envKeyColor   = "EVO_COLOR"
	envKeyVerbose = "EVO_VERBOSE"
	envKeyDebug   = "EVO_DEBUG"
	envKeyNoColor = "NO_COLOR"
)

const (
	envOutputHuman         = "human"
	envOutputPlain         = "plain"
	envOutputJSON          = "json"
	envOutputJSONL         = "jsonl"
	envOutputStreamJSON    = "stream-json"
	envOutputStreamJSONAlt = "stream_json"

	envColorAuto   = "auto"
	envColorAlways = "always"
	envColorNever  = "never"

	envVerboseOn = "1"

	envDebugInfo  = "info"
	envDebugDebug = "debug"
	envDebugTrace = "trace"
)

// Projection selects presentation encoding. Independent of Format, which
// only routes streams (stdout vs stderr). The zero value is unspecified:
// Init fills it from EVO_OUTPUT when set, otherwise human TTY/plain inference.
type Projection int

const (
	// ProjectionHuman is ordinary TTY/plain inference. Non-zero so an
	// explicit Config{Projection: ProjectionHuman} wins over EVO_OUTPUT.
	ProjectionHuman Projection = iota + 1
	// ProjectionPlain forces the durable report (no live region).
	ProjectionPlain
	// ProjectionJSON writes a JSONDocument at Finish.
	ProjectionJSON
	// ProjectionJSONL writes Event JSON Lines at Finish.
	ProjectionJSONL
	// ProjectionStreamJSON writes one EventJSON line at each journal append.
	ProjectionStreamJSON
)

func (p Projection) forcesPlain() bool {
	switch p {
	case ProjectionPlain, ProjectionJSON, ProjectionJSONL, ProjectionStreamJSON:
		return true
	default:
		return false
	}
}

func (p Projection) suppressesHuman() bool {
	switch p {
	case ProjectionJSON, ProjectionJSONL, ProjectionStreamJSON:
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
func SwapLookupEnv(fn func(string) string) func() {
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
	if c.Projection == 0 {
		if p, ok := parseOutputEnv(lookupEnv(envKeyOutput)); ok {
			c = selectOutputEnv(c, p)
		}
	}
	if c.Projection.forcesPlain() {
		c.Plain = true
	}
	if c.Color == ColorAuto {
		if mode, ok := parseColorEnv(lookupEnv(envKeyColor)); ok {
			c.Color = mode
		}
	}
	if c.Verbosity == VerbosityNormal && lookupEnv(envKeyVerbose) == envVerboseOn {
		c.Verbosity = VerbosityVerbose
	}
	if c.Debug.Level == LevelUnset {
		if lvl, ok := parseDebugEnv(lookupEnv(envKeyDebug)); ok {
			c.Debug.Level = lvl
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
func selectOutputEnv(c Config, p Projection) Config {
	switch {
	case c.Format == FormatHuman && p == ProjectionJSON:
		c.Format = FormatJSON
	case c.Format == FormatHuman && p == ProjectionJSONL:
		c.Format = FormatJSONL
	default:
		c.Projection = p
	}
	return c
}

func parseOutputEnv(v string) (Projection, bool) {
	switch strings.TrimSpace(v) {
	case envOutputHuman:
		return ProjectionHuman, true
	case envOutputPlain:
		return ProjectionPlain, true
	case envOutputJSON:
		return ProjectionJSON, true
	case envOutputJSONL:
		return ProjectionJSONL, true
	case envOutputStreamJSON, envOutputStreamJSONAlt:
		return ProjectionStreamJSON, true
	default:
		return 0, false
	}
}

func parseColorEnv(v string) (ColorMode, bool) {
	switch strings.TrimSpace(v) {
	case envColorAuto:
		return ColorAuto, true
	case envColorAlways:
		return ColorAlways, true
	case envColorNever:
		return ColorNever, true
	default:
		return 0, false
	}
}

func parseDebugEnv(v string) (LogLevel, bool) {
	switch strings.TrimSpace(v) {
	case envDebugInfo:
		return LevelInfo, true
	case envDebugDebug:
		return LevelDebug, true
	case envDebugTrace:
		return LevelTrace, true
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
	ProblemCodeDuplicateSiblingName = "duplicate-sibling-name"
	// ProblemCodeVerificationUnsatisfied marks a Task whose post-Define
	// Verify observed that the desired state was not reached (§9.1): the
	// callback ran but its intended effect could not be confirmed.
	ProblemCodeVerificationUnsatisfied = "verification-unsatisfied"
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
func (t *TaskHandle) Key(key string) *TaskHandle {
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
		o.recordMisuseFor(st.name, ErrKeyAfterDefine)
		return t
	}
	if o.graph.Rekey(st.node, txt.Text(key)) == graph.KeyTaken {
		o.recordMisuse(ErrDuplicateKey)
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
func (o *Output) failDuplicateSiblingLocked(col *tasksState, kind graph.EntityKind, name string) error {
	h := o.addTaskLocked(name, col)
	rejected := fmt.Errorf("%w: %s", ErrDuplicateSiblingName, name)
	st := o.taskStates[h.id]
	if st == nil {
		return rejected
	}
	st.node.Rec.AppendProblems(Problem{
		Code:    ProblemCodeDuplicateSiblingName,
		Summary: fmt.Sprintf("duplicate %s name", kind),
	})
	o.settleLocked(st, Failed)
	o.recordMisuseFor(name, ErrDuplicateSiblingName)
	return rejected
}

// --- folded from machine.go ---

func writeMachinePresentation(w io.Writer, snap Snapshot, events []Event, proj Projection, misuse error) error {
	var body []byte
	var err error
	switch proj {
	case ProjectionJSON:
		body, err = wire.EncodeJSON(snap)
	case ProjectionJSONL:
		body, err = wire.EncodeJSONL(events)
	default:
		return misuse
	}
	if err != nil {
		err = fmt.Errorf("%w: %v", ErrRenderer, err)
		if misuse == nil {
			return err
		}
		return errors.Join(misuse, err)
	}
	if w != nil && len(body) > 0 {
		if _, werr := w.Write(body); werr != nil {
			werr = fmt.Errorf("%w: %v", ErrRenderer, werr)
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

func (o *Output) ensureOpen() error {
	if o.closed || o.finishing || o.finished {
		return ErrClosed
	}
	return nil
}

// Close is idempotent cleanup; best-effort Finish when needed. The first
// call tears down and returns its error; a concurrent or later call waits
// for that teardown to end and returns nil.
func (o *Output) Close() error {
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
		_ = o.Finish()
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
func (o *Output) beginRunContext(ctx context.Context) context.Context {
	return o.graph.BeginRunContext(ctx)
}

// Context reports the run's cancellation signal. It is cancelled when the
// run is interrupted (SIGINT/SIGTERM) and when the Output closes, so work
// that does I/O can select on it and stop instead of running on past the ^C
// that was supposed to end it. A nil Output reports a never-cancelled
// context so a caller never has to nil-check before selecting.
func (o *Output) Context() context.Context {
	if o == nil {
		return context.Background()
	}
	return o.graph.Context()
}

// Conclusion returns the computed conclusion after Finish.
func (o *Output) Conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok := o.rec.Conclusion(); ok {
		return c
	}
	snap := o.snapshotLocked()
	c := record.InferConclusion(snap)
	o.explainCancellationLocked(&c)
	record.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
	return c
}

// cancelCauseUser is the cause an interrupt signal records: the person at
// the terminal stopped the run.
const cancelCauseUser = "by user"

// cancelCauseClose is the cause a Close records on a Confirm still waiting.
const cancelCauseClose = "run closed"

// explainCancellationLocked names the cancellation cause on a cancelled
// conclusion. Any other outcome keeps its own Explanation untouched.
func (o *Output) explainCancellationLocked(c *record.Conclusion) {
	if c.State == record.StateCancelled && c.Explanation == "" {
		c.Explanation = o.graph.CancelCause()
	}
}

// --- folded from aliases.go ---

// Domain types live in internal/core / internal/txt. Engine aliases them so
// implementation files keep unqualified names; the public evo package aliases
// the same underlying types directly (not through this package).

type EntityState = record.EntityState

const (
	Pending    = record.Pending
	Running    = record.Running
	Done       = record.Done
	Blocked    = record.Blocked
	Failed     = record.Failed
	Skipped    = record.Skipped
	Cancelled  = record.Cancelled
	Empty      = record.Empty
	Incomplete = record.Incomplete
	NotStarted = record.NotStarted
)

type ConclusionState = record.ConclusionState

const (
	StateReady     = record.StateReady
	StateChanged   = record.StateChanged
	StateWarning   = record.StateWarning
	StateBlocked   = record.StateBlocked
	StateFailed    = record.StateFailed
	StateCancelled = record.StateCancelled
	StatePlanned   = record.StatePlanned
)

type ProgressKind = record.ProgressKind

const (
	Indeterminate = record.Indeterminate
	Determinate   = record.Determinate
	BytesKind     = record.BytesKind
)

type (
	Progress   = record.Progress
	FactRecord = record.Fact
	Event      = record.Event
)

const EventSchemaVersion = record.EventSchemaVersion

type (
	Result          = record.Result
	Snapshot        = record.Snapshot
	TaskSnapshot    = record.TaskSnapshot
	TaxonomyRecord  = record.TaxonomyRecord
	TasksSnapshot   = record.TasksSnapshot
	ChangesSnapshot = record.ChangesSnapshot
	PlanSnapshot    = record.PlanSnapshot
	EffectRecord    = record.EffectRecord
	Conclusion      = record.Conclusion
)

const (
	ExitOK        = record.ExitOK
	ExitBlocked   = record.ExitBlocked
	ExitFailed    = record.ExitFailed
	ExitCancelled = record.ExitCancelled
)

type Resolution = record.Resolution

const (
	ResolutionExecuted         = record.ResolutionExecuted
	ResolutionAlreadySatisfied = record.ResolutionAlreadySatisfied
	ResolutionNoWork           = record.ResolutionNoWork
)

type (
	EvidencePhase = record.EvidencePhase
	TaskEvidence  = record.TaskEvidence
)

type (
	Problem        = record.Problem
	SourceLocation = record.SourceLocation
	Attachment     = record.Attachment
	Field          = record.Field
	Action         = record.Action
	CommandSpec    = record.CommandSpec
	GlyphProfile   = txt.GlyphProfile
)

const (
	GlyphsAuto    = txt.GlyphsAuto
	GlyphsUnicode = txt.GlyphsUnicode
	GlyphsASCII   = txt.GlyphsASCII
)

// --- folded from option.go ---

// Option configures an Output.
type Option interface {
	apply(*config)
}

type config struct {
	subject           string
	primary           io.Writer
	diagnostic        io.Writer
	result            io.Writer // domain payload (FormatData); never used for presentation
	plain             bool
	projection        Projection
	noColor           bool
	width             int
	clock             TimeSource
	visibilityDelay   time.Duration
	maxFrameRate      int
	strict            bool
	terminal          TerminalDriver
	debugLevel        LogLevel
	debugAddSource    bool
	debugPresentation DebugPresentation
	debugPane         debugPaneConfig
	redactor          Redactor
	maxEntities       int
	maxEvents         int
	maxConcurrency    int
	extraWriters      []io.Writer
	verbosity         Verbosity
	// stateDir/appID mirror Config.StateDir/Config.AppID (spec §11.3):
	// stateDir, when non-empty, is the exact manifest state directory;
	// appID overrides the derived application id used in the default
	// cache-dir manifest path derivation.
	stateDir string
	appID    string
	// processRunner is the facade evo.Exec spawns child processes through
	// (spec §8.4). Defaults to osProcessRunner{} in resolveConfig.
	processRunner ProcessRunner
	// fileFS is the facade evo.File performs filesystem I/O through (spec
	// §8.2). Defaults to osFileFS{} in newOutput.
	fileFS FileFS
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
	glyphs GlyphProfile
	// samePrimaryAsTerminal records that the live Terminal driver was built
	// around the same underlying writer as primary (Config's default-construction
	// path: to(c.Stdout) and withTerminal(ansi-over-c.Stdout) target one stream).
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
	wireFormat Format
	wireStream io.Writer
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// To sets the primary human writer.
func to(w io.Writer) Option {
	return optionFunc(func(c *config) { c.primary = w })
}

// Diagnostics sets the diagnostic writer for Debug history and Capture mirrors.
// When set and distinct from the primary writer (To), Debug lines are not also
// written to the human primary stream — use dual-stream for LaunchAgent /
// data-command layouts (human on stdout, diagnostics on stderr).
func withDiagnostics(w io.Writer) Option {
	return optionFunc(func(c *config) { c.diagnostic = w })
}

// ResultStream sets the domain-payload writer (see Output.ResultWriter).
// Presentation never writes here. FormatData defaults this to Config.Stdout.
func resultStream(w io.Writer) Option {
	return optionFunc(func(c *config) {
		if w != nil {
			c.result = w
		}
	})
}

// wireFormatOption selects the v2 machine wire encoder Finish/appendEventLocked
// write alongside ordinary human presentation (FormatJSON/FormatJSONL only —
// see configToOptions).
func wireFormatOption(f Format) Option {
	return optionFunc(func(c *config) { c.wireFormat = f })
}

// wireStreamOption sets where the v2 wire encoder writes (Config.Stdout).
func wireStreamOption(w io.Writer) Option {
	return optionFunc(func(c *config) { c.wireStream = w })
}

// Plain forces final-report projection (no live spinner region).
// Semantic color is still emitted unless NoColor is set.
func plain() Option {
	return optionFunc(func(c *config) { c.plain = true })
}

func withProjection(p Projection) Option {
	return optionFunc(func(c *config) { c.projection = p })
}

// NoColor disables color.
func withNoColor() Option {
	return optionFunc(func(c *config) { c.noColor = true })
}

// Width sets the terminal width in columns.
func withWidth(columns int) Option {
	return optionFunc(func(c *config) { c.width = columns })
}

// Clock injects a time source facade.
func withClock(ts TimeSource) Option {
	return optionFunc(func(c *config) { c.clock = ts })
}

// VisibilityDelay sets how long live activity must persist before the first
// interactive paint (default 80ms). Zero paints immediately. Prevents Phase→fast
// Done spinner flash (H.2). Domain TimeSource is used for the threshold.
func visibilityDelay(delay time.Duration) Option {
	return optionFunc(func(c *config) { c.visibilityDelay = delay })
}

// MaxFrameRate caps interactive redraws per second.
func maxFrameRate(framesPerSecond int) Option {
	return optionFunc(func(c *config) { c.maxFrameRate = framesPerSecond })
}

// Strict enables panic-on-misuse for tests.
func strict() Option {
	return optionFunc(func(c *config) { c.strict = true })
}

// DryRun declares this run a dry run: evo.Effect, evo.File, and
// evo.Exec render as
// [planned] rows with imperative verbs instead of [changed] rows with
// past-tense verbs. Set once via Config.DryRun in ordinary application code;
// this Option exists for the advanced NewWithOptions surface and tests.
func dryRun() Option {
	return optionFunc(func(c *config) { c.dryRun = true })
}

// dryRunHeader is Init's unexported plumbing for Config.Subject: it lets a
// dry-run's opening marker merge the subject onto its own one-line
// announcement instead of the two separate durable lines a bare Println
// would produce. Not part of the public Option surface — Config.Subject and
// Output.Subject stay the caller's only entry points.
func dryRunHeader(text string) Option {
	return optionFunc(func(c *config) { c.dryRunHeaderText = text })
}

// preview declares the planned tense a preview before a confirm gate rather
// than a dry run — same skipped callbacks and same [planned] ledger, header
// without the tag. Set via Config.Preview; this Option is construction
// plumbing only.
func preview() Option {
	return optionFunc(func(c *config) { c.preview = true })
}

// Stdin injects the reader Confirm reads answers from (facade rule — no
// direct os.Stdin read in Confirm's logic). Default os.Stdin.
func stdin(r io.Reader) Option {
	return optionFunc(func(c *config) { c.stdin = r })
}

// Terminal injects a terminal driver (interactive projection; v0.2).
func withTerminal(driver TerminalDriver) Option {
	return optionFunc(func(c *config) { c.terminal = driver })
}

// withPrimarySharesTerminal marks that the just-configured Terminal driver
// writes to the same stream as primary (To). Only configToOptions calls this
// — it knows both writers at construction time, so Finish never has to guess
// stream identity from an fd comparison.
func withPrimarySharesTerminal() Option {
	return optionFunc(func(c *config) { c.samePrimaryAsTerminal = true })
}

// withDiagnosticSharesTerminal marks that the Diagnostics writer resolves to
// the same physical terminal device as the live region, even when it is a
// distinct io.Writer from primary. Only configToOptions calls this — see
// config.diagnosticSharesTerminal.
func withDiagnosticSharesTerminal() Option {
	return optionFunc(func(c *config) { c.diagnosticSharesTerminal = true })
}

// LogLevel is a diagnostic severity.
//
// The zero value is LevelUnset (Config resolves it to LevelInfo). Named levels
// start at LevelTrace so ordinary Config{Debug: DebugConfig{Level: LevelTrace}}
// is expressible without falling through the default path.
type LogLevel int

const (
	// LevelUnset is the zero value. Config resolve maps it to LevelInfo.
	LevelUnset LogLevel = iota
	// LevelTrace is the most verbose journal level selectable via Config.
	LevelTrace
	// LevelDebug enables Debug journal lines (and Capture MirrorToDebug).
	LevelDebug
	// LevelInfo is the ordinary default (Debug journal suppressed).
	LevelInfo
	// LevelWarn is reserved for future warn-threshold filtering.
	LevelWarn
	// LevelError is reserved for future error-threshold filtering.
	LevelError
)

// DebugLevel sets the minimum debug emission level.
// Pass LevelTrace or LevelDebug to surface Debug journal lines.
func debugLevel(level LogLevel) Option {
	return optionFunc(func(c *config) {
		if level == LevelUnset {
			c.debugLevel = LevelInfo
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
func debugAddSource() Option {
	return optionFunc(func(c *config) { c.debugAddSource = true })
}

// TerminalDriver is the exclusive owner of terminal control sequences.
type TerminalDriver = terminal.Driver

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
func maxEntities(n int) Option {
	return optionFunc(func(c *config) { c.maxEntities = n })
}

// MaxEvents caps durable journal events; when exceeded, oldest non-critical
// events are dropped so critical terminal events are retained (CON-008).
func maxEvents(n int) Option {
	return optionFunc(func(c *config) { c.maxEvents = n })
}

func maxConcurrency(n int) Option {
	return optionFunc(func(c *config) { c.maxConcurrency = n })
}

// withStateDir mirrors Config.StateDir (spec §11.3).
func withStateDir(dir string) Option {
	return optionFunc(func(c *config) { c.stateDir = dir })
}

// withAppID mirrors Config.AppID (spec §11.3).
func withAppID(id string) Option {
	return optionFunc(func(c *config) { c.appID = id })
}

// withProcessRunner injects the facade evo.Exec spawns every child process
// through (spec §8.4) — testkit installs a scripted fake so Exec's skip
// protocol, capture, and cancellation are provable without a real
// subprocess.
func withProcessRunner(r ProcessRunner) Option {
	return optionFunc(func(c *config) { c.processRunner = r })
}

// withFileFS injects the facade evo.File performs filesystem I/O through
// (spec §8.2) — testkit installs a fake so a reconciliation outcome (e.g.
// a chmod failure) is provable without a real disk permission.
func withFileFS(fsys FileFS) Option {
	return optionFunc(func(c *config) { c.fileFS = fsys })
}

// AlsoWrite adds an additional human projection writer. On Finish, each writer
// receives the plain projection; failures on one do not skip the others (CON-009).
func alsoWrite(w io.Writer) Option {
	return optionFunc(func(c *config) {
		if w != nil {
			c.extraWriters = append(c.extraWriters, w)
		}
	})
}

// --- folded from export.go ---

// Exported aliases for the public evo facade.

func To(w io.Writer) Option                  { return to(w) }
func Diagnostics(w io.Writer) Option         { return withDiagnostics(w) }
func ResultStream(w io.Writer) Option        { return resultStream(w) }
func Plain() Option                          { return plain() }
func NoColor() Option                        { return withNoColor() }
func Width(columns int) Option               { return withWidth(columns) }
func Clock(ts TimeSource) Option             { return withClock(ts) }
func VisibilityDelay(d time.Duration) Option { return visibilityDelay(d) }
func MaxFrameRate(n int) Option              { return maxFrameRate(n) }
func Strict() Option                         { return strict() }
func DryRun() Option                         { return dryRun() }
func Stdin(r io.Reader) Option               { return stdin(r) }
func Terminal(driver TerminalDriver) Option  { return withTerminal(driver) }
func DebugLevel(level LogLevel) Option       { return debugLevel(level) }
func DebugAddSource() Option                 { return debugAddSource() }
func MaxEntities(n int) Option               { return maxEntities(n) }
func MaxEvents(n int) Option                 { return maxEvents(n) }
func AlsoWrite(w io.Writer) Option           { return alsoWrite(w) }
func Redact(r Redactor) Option               { return redact(r) }
func Runner(r ProcessRunner) Option          { return withProcessRunner(r) }
func DataProjection() Option                 { return dataProjection() }
func ExternalProjection() Option             { return externalProjection() }
func DebugHistory() Option                   { return debugHistory() }
func DebugPane(opts ...DebugPaneOption) Option {
	return debugPane(opts...)
}
func KeepLastLines(n int) CaptureOption    { return keepLastLines(n) }
func MaxCaptureBytes(n int) CaptureOption  { return maxCaptureBytes(n) }
func MirrorToDiagnostics() CaptureOption   { return mirrorToDiagnostics() }
func MirrorToDebug() CaptureOption         { return mirrorToDebug() }
func PaneHeight(lines int) DebugPaneOption { return paneHeight(lines) }
func NewestFirst() DebugPaneOption         { return newestFirst() }
func OldestFirst() DebugPaneOption         { return oldestFirst() }
func PreserveDebugTail() DebugPaneOption   { return preserveDebugTail() }
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}

type (
	Capture      = evidence
	SystemClock  = systemClock
	FixedClock   = fixedClock
	NoopRedactor = noopRedactor
)

// Test helpers reachable through the evo type alias (root export_test.go
// cannot attach methods to engine types).

func (t *TaskHandle) RunForTest(cmd *process.Cmd) error {
	if t == nil || t.out == nil {
		return nil
	}
	return t.run(cmd)
}

func (t *TaskHandle) StepForTest(completed, total int, name string) *TaskHandle {
	return t.Progress(completed, total).Doing(name)
}

func (t *TaskHandle) CaptureForTest(opts ...CaptureOption) *evidence {
	return t.Capture(opts...)
}
func (o *Output) CaptureForTest(opts ...CaptureOption) *evidence { return o.capture(opts...) }
func (o *Output) Events() []Event                                { return o.copyEvents() }
func (o *Output) DebugForTest(message string, fields ...Field) {
	o.debug(message, fields...)
}

func (o *Output) AlsoWriteForTest(w io.Writer) {
	if w == nil {
		return
	}
	o.cfg.extraWriters = append(o.cfg.extraWriters, w)
}

func (t *TaskHandle) SkippedWithErrs(reason TaxonomyReason, name string, errs ...error) {
	t.recordTaxonomy(reason, name, dispositionSkip, errs)
}
func (o *Output) SetDiagnosticSharesTerminalForTest() { o.cfg.diagnosticSharesTerminal = true }
func (o *Output) DropDiagnosticForTest()              { o.cfg.diagnostic = nil }
func (o *Output) ForceLiveVisibleForTest() {
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
func (o *Output) DebugWriterForTest() io.WriteCloser           { return o.debugWriter() }
func (o *Output) DeclareDryRunForTest()                        { o.declareDryRun() }
func (o *Output) AboutForTest(text string)                     { o.about(text) }
func (o *Output) SubjectForTest(text string)                   { o.subject(text) }
func (o *Output) SlogHandlerForTest() slog.Handler             { return o.slogHandler() }
func (o *Output) NextSelfForTest(args ...string) ProblemOption { return o.nextSelf(args...) }
func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	return t.skip(reason, args...)
}
func (o *Output) AtForTest(visibility Visibility) *Printer { return o.at(visibility) }
func (o *Output) SchedulerMaxObserved() int {
	return o.graph.MaxObserved()
}

func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return Default().reasonGetOrCreate(name, opts...)
}

// --- folded from basis.go ---

// Fingerprint observes one Basis input's current content identity
// (spec §11.1). Implementations must not mutate the world.
type Fingerprint = freshness.Fingerprint

// FingerprintValue is one Fingerprint's observed identity: a stable
// machine Kind, a stable non-secret Key, and a SHA-256 Digest.
type FingerprintValue = freshness.FingerprintValue

// FSPath fingerprints the filesystem content at path: a regular file's
// type-marked byte digest, a directory's deterministic Merkle digest over
// sorted entries, a symlink's target text (never followed), or a stable
// "missing" digest. Permission, mtime, and owner changes never change the
// digest; an unreadable path is an error, never reported as missing.
func FSPath(path string) Fingerprint { return freshness.FSPath(path) }

// Value fingerprints a caller-supplied scalar (string, bool, any signed/
// unsigned integer, any finite float, time.Time, or []byte) under a stable,
// safe name. Only the digest and name are ever persisted, never the raw
// value. Any other type is an error when the Fingerprint is taken.
func Value(name string, v any) Fingerprint { return freshness.Value(name, v) }

// App fingerprints the running application itself: the executable's bytes
// when readable, else its Go build ID, else an error when the Fingerprint
// is taken. Include it in a Task's Basis only when the application's own
// implementation is a semantic input to that Task's result.
func App() Fingerprint { return freshness.App() }

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
func detectCapabilities(opts ...Option) capabilityProfile {
	cfg := config{
		width:      defaultWidth,
		debugLevel: LevelInfo,
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
type changeHost struct{ out *Output }

// DryRun reports whether this Output is configured for dry-run/preview
// tense — the same flag evo.Effect and evo.File render by.
func (o *Output) DryRun() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cfg.dryRun
}

// DryRun reports whether the Run plans its changes instead of making them.
func (h changeHost) DryRun() bool { return h.out.DryRun() }

// FS is the filesystem File performs I/O through — real os calls in
// ordinary use, a scripted fake under test (facade rule).
func (h changeHost) FS() FileFS {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	return h.out.cfg.fileFS
}

// taskLedger is one Task's ledger section, resolved once.
type taskLedger struct {
	out    *Output
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
func Pluralize(quantity int64, singular string) string {
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
type CaptureStream = process.Stream

const (
	CaptureStreamCombined = process.StreamCombined
	CaptureStreamStdout   = process.StreamStdout
	CaptureStreamStderr   = process.StreamStderr
)

// evidence is the retained/redacted process-output sink owned by a Task
// (preferred) or Output. The bounded ring lives in process; this type owns
// sanitization, redaction, mirroring, and Task attachment.
type evidence struct {
	out      *Output
	taskID   string
	taskName string
	ring     *process.Ring
	maxLines int
	maxBytes int

	mirrorDiag  bool
	mirrorDebug bool
	stream      CaptureStream
	parent      *evidence
	onLine      func(text string)
}

// CaptureOption configures Capture.
type CaptureOption interface {
	applyCapture(*evidence)
}

type captureOptionFunc func(*evidence)

func (f captureOptionFunc) applyCapture(c *evidence) { f(c) }

func keepLastLines(n int) CaptureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

func maxCaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

func mirrorToDiagnostics() CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDiag = true })
}

func mirrorToDebug() CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDebug = true })
}

func activityFeed(fn func(text string)) CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.onLine = fn })
}

func (t *TaskHandle) Capture(opts ...CaptureOption) *evidence {
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

func (o *Output) capture(opts ...CaptureOption) *evidence {
	return newCapture(o, "", "", opts...)
}

func newCapture(out *Output, taskID, taskName string, opts ...CaptureOption) *evidence {
	c := &evidence{
		out:         out,
		taskID:      taskID,
		taskName:    taskName,
		maxLines:    defaultCaptureLines,
		maxBytes:    defaultCaptureBytes,
		mirrorDiag:  false,
		mirrorDebug: false,
		stream:      CaptureStreamCombined,
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
	return &evidence{out: c.out, parent: c, stream: CaptureStreamStdout, ring: c.ring}
}

func (c *evidence) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: CaptureStreamStderr, ring: c.ring}
}

func (c *evidence) Write(p []byte) (int, error) {
	root := c.root()
	if root == nil || root.ring == nil {
		return len(p), nil
	}
	stream := CaptureStreamCombined
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
		CaptureStreamCombined, CaptureStreamStdout, CaptureStreamStderr,
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
	lines, truncated := c.snapshotTexts(CaptureStreamCombined, 0)
	return process.JoinLines(lines, truncated)
}

func (c *evidence) streamText(stream CaptureStream) string {
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
	prefer := CaptureStreamCombined
	if c.parent != nil {
		prefer = c.stream
	} else if root.ring.PreferStderr() || root.ring.HasStream(CaptureStreamStderr) {
		prefer = CaptureStreamStderr
	}
	texts, truncated := root.ring.Texts(prefer, 0, root.normalizeCaptureLine)
	if len(texts) == 0 {
		texts, truncated = root.ring.Texts(CaptureStreamCombined, 0, root.normalizeCaptureLine)
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

func (c *evidence) snapshotTexts(stream CaptureStream, limit int) ([]string, bool) {
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

func (o *Output) mirrorCaptureLine(mirrorDiag, mirrorDebug bool, taskName, line string) {
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
	allowDebug := o.cfg.debugLevel <= LevelDebug
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
		o.debug(line, Field{Key: "task", Value: taskName})
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

func (t *TaskHandle) submitWork(run func() error) {
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
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		o.mu.Unlock()
		return
	case graph.AlreadySubmitted:
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	o.followRecordLocked()
	o.mu.Unlock()
	o.graph.Kick()
}

// workOf is what the graph runs for st: the callback, and the engine's own
// steps around it.
func (o *Output) workOf(st *taskState, run func() error) graph.Work {
	return graph.Work{
		Started:  func() bool { return o.taskStarted(st) },
		Run:      run,
		Observed: func(err error) { o.resolveObserved(st, err) },
		Panicked: st.handle.failScheduled,
	}
}

// taskStarted is the render work a claimed Task owes: the eligibility event,
// the move to Running, the snapshot version and a forced paint, within the
// live render budget, since a start is the spinner FP-005 requires before the
// check. It reports whether the Task began: false when an interrupt settled
// it between its claim and this step, so the scheduler skips its callback.
func (o *Output) taskStarted(st *taskState) bool {
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
	if st.node.Rec.State() == Pending {
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
// TaskHandle.finish). A ratified proposal supplies the row's summary — the
// caller's own words, now backed by an observation. A returned error
// replaces the proposal: it was never a resolution, so it earns no
// "resolve each task once" misuse line (E-113).
func (o *Output) resolveObserved(st *taskState, err error) {
	observation := o.graph.Observe(st.node, err)
	switch observation.Kind {
	case graph.ObservedFailure:
		st.handle.failScheduled(observation.Failure)
	case graph.ObservedProposal:
		proposal := observation.Proposal
		st.handle.resolveScheduled(proposal.State, proposal.Summary, proposal.Problems)
	default:
		st.handle.doneScheduled()
	}
}

// drainScheduler runs the queue to empty.
func (o *Output) drainScheduler() { o.graph.Drain() }

// --- folded from file_manifest.go ---

// emitManifestWarningOnce surfaces a manifest Store's safe cache-miss
// warning (corrupt/unknown prior manifest, spec §11.3) as a run-scoped Fact
// exactly once per Run — never as an error, since a miss is always safe to
// continue past.
func (o *Output) emitManifestWarningOnce(w error) {
	if w == nil || !o.manifest.FirstMissWarning() {
		return
	}
	o.Fact("manifest", w.Error())
}

// taskManifestKeyLocked returns taskID's stable manifest Task key (§3.1's
// stableKey, already computed at declaration) and the next pending
// operation ordinal within it. Callers must already hold o.mu.
func (o *Output) taskManifestKeyLocked(taskID string) (key string, ordinal int, ok bool) {
	st := o.taskStates[taskID]
	if st == nil {
		return "", 0, false
	}
	return st.ManifestKey(), o.taskFreshness.OperationCount(st.TaskID()), true
}

// appendManifestOperationLocked records rec as taskID's next pending
// operation, committed only if/when the Task itself settles Done (spec
// §8.2/§11.3). Callers must already hold o.mu.
func (o *Output) appendManifestOperationLocked(taskID string, rec freshness.OperationRecord) {
	if st := o.taskStates[taskID]; st != nil {
		o.taskFreshness.AppendOperation(st.TaskID(), rec)
	}
}

// planManifestOperationLocked notes that taskID's next operation was found
// stale and, in a planned Run, left unapplied; it holds its place in the
// Task's order without a record. Callers must already hold o.mu.
func (o *Output) planManifestOperationLocked(taskID string) {
	if st := o.taskStates[taskID]; st != nil {
		o.taskFreshness.PlanOperation(st.TaskID())
	}
}

// commitManifestTaskLocked persists taskID's accumulated operations as this
// Run's truth (spec §11.3) once the Task has settled Done; see
// freshness.TaskTable.Commit. Callers must already hold o.mu.
func (o *Output) commitManifestTaskLocked(ctx context.Context, taskID string) {
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
func (o *Output) saveManifest() {
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
func (o *Output) warnManifestUnsavedLocked(err error) {
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
func (o *Output) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: o.changes.Workspace()}
}

// manifestFor returns this Run's manifest Store, opening it on first use
// (spec §11.3). A manifest miss or open failure degrades this Run to
// live-filesystem-only File behavior; see freshness.ManifestSession.Store.
func (o *Output) manifestFor(ctx context.Context) (*freshness.ManifestStore, error) {
	store, err := o.manifest.Store(ctx)
	if errors.Is(err, freshness.ErrManifestSessionClosed) {
		return nil, ErrClosed
	}
	return store, err
}

// runModeLocked is whether this Run applies its work or only plans it, as
// freshness asks. Callers must already hold o.mu.
func (o *Output) runModeLocked() freshness.RunMode {
	if o.cfg.dryRun {
		return freshness.RunPlans
	}
	return freshness.RunApplies
}

// --- folded from freshness_task.go ---

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FingerprintBasis.
type BasisSource = freshness.BasisSource

// FingerprintBasis observes a Fingerprint (FSPath, Value, App).
func FingerprintBasis(f Fingerprint) BasisSource { return freshness.FingerprintBasis(f) }

// basisIsCurrent judges this Task's Basis as it starts (see
// freshness.TaskTable.JudgeBasis) and narrates the verdict. A Task with no
// Basis is never current.
func (t *TaskHandle) basisIsCurrent(ctx context.Context) (bool, error) {
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
func (o *Output) interrupt(reason string) {
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
// blocks on stdin) and its abort channel — not TaskHandle.Cancel — is what
// unblocks the stdin read. Since a Confirm gate is an ordinary Task while
// its answer is pending, the generic Pending-task fallback would otherwise
// resolve it to Cancelled without ever closing that channel, leaving
// readConfirmLine blocked forever.
func (o *Output) cancelActive(reason string) {
	o.mu.Lock()
	if o.cancelPendingConfirmLocked(reason) {
		o.mu.Unlock()
		return
	}
	running := make([]*TaskHandle, 0, len(o.tasks))
	for _, t := range o.tasks {
		if t.node.Rec.State() == Running {
			running = append(running, t.handle)
		}
	}
	if len(running) > 0 {
		o.mu.Unlock()
		for _, t := range running {
			t.Cancel(reason)
		}
		return
	}
	// Nothing has reached Running yet: the earliest-declared Pending task is
	// the one about to run next (evo-rec.md "one Running child" — pending
	// siblings are named and idle, waiting their turn), so an interrupt
	// before any evidence still cancels that task rather than falling
	// through to Output-level cancel.
	var active *TaskHandle
	for _, t := range o.tasks {
		if t.node.Rec.State() == Pending {
			active = t.handle
			break
		}
	}
	if active != nil {
		o.mu.Unlock()
		active.Cancel(reason)
		return
	}
	o.mu.Unlock()
	o.Cancel(reason)
}

// cancelPendingConfirmLocked cancels one pending Confirm gate, if any, so ^C
// at a "[y/N]" prompt unblocks Confirm's stdin read and resolves the gate as
// Cancelled — never Blocked "declined" (a human "n" and an interrupt are
// distinct outcomes). Reports whether a gate was cancelled.
func (o *Output) cancelPendingConfirmLocked(reason string) bool {
	id, ok := o.graph.AbortPending()
	if !ok {
		return false
	}
	if st := o.taskStates[id]; st != nil && !record.IsTerminalTask(st.node.Rec.State()) {
		st.node.Rec.SetSummary(reason)
		o.settleLocked(st, Cancelled)
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

func (o *Output) recordMisuse(err error) {
	o.misuse.Record(err)
}

func (o *Output) recordMisuseFor(subject string, err error) {
	o.misuse.RecordMisuseFor(subject, err)
}

func (o *Output) firstMisuse() error {
	return o.misuse.First()
}

// appendMisuseLineLocked renders the one required line for the first
// recorded misuse — an exit code may never disagree with everything the
// caller saw printed (beginner-1). The line is the recorder's corrective
// sentence for the recorded sentinel, never the raw "evo: ..." sentinel
// text (release-gate round 4 finding 2): machine detail stays in
// JSON/debug, the human stream gets told what to do next.
func (o *Output) appendMisuseLineLocked() {
	hint := o.misuse.Hint()
	glyph := txt.StyleGlyph(misuseGlyph, txt.SGRYellow, !o.cfg.noColor)
	o.rec.AppendLine(fmt.Sprintf("%s  %s", glyph, hint))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Fail/Block/Warn/Cancel/Skipped) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Output) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	o.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}

// Err returns the first recorded misuse error, if any.
func (o *Output) Err() error {
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
func redact(r Redactor) Option {
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
func (o *Output) redactString(s string) string {
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
func Fact(name, value string) {
	Default().Fact(name, value)
}

// Fact accumulates a run-scoped discovered name/value annotation (P8
// symmetry with TaskHandle.Fact) — information about the run, fire-and-
// forget: it both renders a durable dim "name  value" line immediately
// (the same "act now" contract Println has) and stores the annotation for
// the structured Snapshot/JSON views. A nil Output is safe and records
// nothing.
func (o *Output) Fact(name, value string) {
	if o == nil {
		return
	}
	f := record.SanitizeFact(FactRecord{Name: txt.Text(name), Value: txt.Text(value)})
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
// TaskHandle.Problem(..., Severity(SeverityWarning)) uses for the
// conclusion's warned modifier.
func (o *Output) warnLocked(p Problem) {
	o.rec.RecordRunWarning(p)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "run.warned", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventWarningRecorded, "", wire.ToProblemDoc(p).EventPayload())
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(o.cfg.glyphs), txt.SGRYellow, !o.cfg.noColor)
	o.writeDurableTextLocked(glyph + " " + p.Summary + "\n")
}

// --- folded from run_signal.go ---

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (wired to SIGINT/SIGTERM by those
// entrypoints) in place of the pre-v0.6 no-context func() error form.
type RunFunc func(context.Context) error

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
// TaskHandle.finish/resolveScheduled) so sequence followers still cascade.
var errVerificationUnsatisfied = errors.New("evo: postcondition not satisfied")

// Verify registers an advanced current-state observation check (§9.1),
// ANDed with any previously registered check on this Task. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define — a call after Define is ignored and recorded as misuse.
//
// Define's own execution wiring (see runDefine) runs every registered
// Verify twice: once before the callback (with the Task already Running) —
// all true skips the callback entirely and resolves the Task successfully
// with ResolutionAlreadySatisfied; any false enters the callback; an
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
func (t *TaskHandle) Verify(fn func(context.Context) (bool, error)) *TaskHandle {
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
// Define returns the same *TaskHandle (ZYS-849 Decisions) so the common
// single-Task shape can be written `return task.Define(fn).Wait()`. This is
// fluent sugar only — it does not change Define's asynchronous submission:
// fn still runs on the scheduler, not inline before Define returns.
//
// A second Define on the same Task is misuse (submitWork's own
// already-submitted guard) — configuration freezes once.
func (t *TaskHandle) Define(fn func(context.Context) error) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	if fn == nil {
		o.recordMisuse(ErrInvalidConfig)
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

	t.submitWork(func() error { return t.runDefine(verifiers, fn) })
	return t
}

// runDefine is Define's Verify-aware execution wiring (§7, §9.1, §29/§30).
// It resolves the Task itself on every path except "callback returned an
// error" and "callback returned success with nothing left to check" — those
// two fall through to the scheduler's own generic resolution
// (executeWork/resolveObserved) via passthroughCallbackOutcome, exactly as a
// Verify-less Define always has.
func (t *TaskHandle) runDefine(verifiers []freshness.Verifier, fn func(context.Context) error) error {
	o := t.out
	scope := graph.NewScope(o.graph, record.TaskID(t.id), o.changes)

	if current, basisErr := t.basisIsCurrent(o.Context()); basisErr != nil {
		t.failScheduled(basisErr.Error())
		return passthroughCallbackOutcome(basisErr)
	} else if current {
		o.setResolution(t.id, ResolutionAlreadySatisfied)
		t.doneScheduled()
		return nil
	}

	if len(verifiers) > 0 {
		allSatisfied, obsErr := evaluateVerifiers(graph.WithScope(o.Context(), scope), o, t.id, verifiers)
		if obsErr != nil {
			o.recordEvidencePhase(t.id, evidencePhaseBefore, true, false)
			t.failScheduled(obsErr.Error())
			return passthroughCallbackOutcome(obsErr)
		}
		o.recordEvidencePhase(t.id, evidencePhaseBefore, true, allSatisfied)
		if allSatisfied {
			o.setResolution(t.id, ResolutionAlreadySatisfied)
			t.doneScheduled()
			return nil
		}
	}

	o.mu.Lock()
	o.emitWireEventLocked(wire.EventDefinitionStarted, t.id, nil)
	o.mu.Unlock()
	callbackErr := fn(graph.WithScope(o.Context(), scope))
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
	o.setResolution(t.id, ResolutionExecuted)
	return nil
}

// checkAfterDefine is runDefine's post-callback evidence step: every
// registered Verify must now hold, or, with none, tracked operations
// supply the After phase. It returns the carrier error when the Task
// already failed.
func (t *TaskHandle) checkAfterDefine(verifiers []freshness.Verifier, scope *graph.Scope[*change.Run]) error {
	o := t.out
	switch {
	case !t.hasPostStateToVerify():
		// The After phase stays unevaluated.
	case len(verifiers) > 0:
		allSatisfied, obsErr := evaluateVerifiers(graph.WithScope(o.Context(), scope), o, t.id, verifiers)
		if obsErr != nil {
			o.recordEvidencePhase(t.id, evidencePhaseAfter, true, false)
			t.failScheduled(obsErr.Error())
			return passthroughCallbackOutcome(obsErr)
		}
		o.recordEvidencePhase(t.id, evidencePhaseAfter, true, allSatisfied)
		if !allSatisfied {
			t.failScheduledWithCode(ProblemCodeVerificationUnsatisfied, "postcondition not satisfied")
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
func (t *TaskHandle) hasPostStateToVerify() bool {
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
func (o *Output) recordOperationsEvidence(taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil || o.taskFreshness.OperationCount(st.TaskID()) == 0 {
		return
	}
	st.node.Rec.RecordAfterEvidence(EvidencePhase{Evaluated: true, Satisfied: true, Source: operationsEvidenceSource})
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
// already-terminal branch (see TaskHandle.finish).
func passthroughCallbackOutcome(err error) error {
	return err
}

// evaluateVerifiers runs taskID's verifiers (see
// freshness.EvaluateVerifiers) and emits each one's outcome as
// verification.observed (spec §38), named by its registration order since
// Verify registers anonymous funcs.
func evaluateVerifiers(ctx context.Context, o *Output, taskID string, verifiers []freshness.Verifier) (allSatisfied bool, err error) {
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
func (o *Output) recordEvidencePhase(taskID string, phase evidencePhaseName, evaluated, satisfied bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil {
		return
	}
	recorded := EvidencePhase{Evaluated: evaluated}
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

// setResolution stores why a Task settled successfully (§29/§30).
func (o *Output) setResolution(taskID string, r Resolution) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskStates[taskID]; st != nil {
		st.node.Rec.SetResolution(r)
		st.markFiling()
	}
}

// failScheduledWithCode is failScheduled with a stable Problem.Code
// attached — the scheduler's own resolution authority (see
// TaskHandle.resolveScheduled), used for outcomes that need a
// machine-matchable code rather than only a rendered summary.
func (t *TaskHandle) failScheduledWithCode(code, summary string) {
	p := record.SanitizeProblem(Problem{Code: code, Summary: txt.Text(summary)})
	t.resolveScheduled(Failed, txt.Text(summary), []Problem{p})
}
