package evo

import (
	"cmp"
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
	"github.com/zachbornheimer/evident-output/internal/project/live"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
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
	out := initHostRun(cfg)
	if ctx == nil {
		ctx = context.Background()
	}
	return runInterruptible(ctx, out, run)
}

// payloadOnStdout is cfg as a host runs it: a human or plain run reserves stdout for Run.ResultWriter's
// payload and presents on stderr, the routing a machine Output already has. An external format keeps its own.
func payloadOnStdout(cfg Config) Config {
	if cfg.format == formatHuman && cfg.Output != JSON && cfg.Output != JSONL {
		cfg.format = formatData
	}
	return cfg
}

// Main runs run under SIGINT and SIGTERM and returns the process exit code; it
// never exits. A signal cancels the run's context and ends the run Cancelled with
// exit code 130, including while declared work still runs after run returned.
// Its arguments are Execute's, without the context Main owns.
func Main(cfg Config, run RunFunc) int {
	out := initHostRun(cfg)
	return runInterruptible(context.Background(), out, run).ExitCode()
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

// selfName is Config.Title, else the binary's basename.
func (o *Run) selfName() string {
	if o.cfg.subject != "" {
		return o.cfg.subject
	}
	return identityFallbackName()
}

// identityFallbackName is the executable's basename or "command"; never a Subject.
func identityFallbackName() string {
	if base := filepath.Base(processArgv0()); base != "." && base != string(filepath.Separator) {
		return base
	}
	return "command"
}

type colorMode int

const (
	colorAuto colorMode = iota
	colorAlways
	colorNever
)

type format int

const (
	formatHuman format = iota
	// FormatData reserves stdout for the domain payload; human output goes to stderr.
	formatData
	formatExternal
	// FormatJSON writes one final v2 "evo.run" document to Stdout at Finish; human output goes to Stderr (spec §32.1).
	formatJSON
	// FormatJSONL streams v2 "evo.event" lines to Stdout plus a final run.finished line; human output goes to Stderr.
	formatJSONL
)

type verbosity int

const (
	verbosityNormal verbosity = iota
	// VerbosityVerbose also projects Verbose messages and expands each Task.Skipped/Kept row into
	// one line per reason. The names live on TaskSnapshot; the wire JSONTask omits them.
	verbosityVerbose
)

type debugConfig struct {
	// Level is the minimum debug journal level; zero resolves to LevelInfo.
	level      logLevel
	view       debugPresentation
	paneHeight int
	// NewestFirst orders the pane (default true when zero-config pane).
	newestFirst    *bool
	preserveAlways bool
	// AddSource resolves each record's call site to source=file.go:line; off by default (LogRecord.PC keeps it).
	addSource bool
}

// Config holds a run's settings, passed to Main or Execute. The zero Config is
// complete: Name defaults to the binary name, and Output to Human on a terminal
// and Plain otherwise. Name titles the run and is the name in the evo.run
// document; Verbose shows hidden Facts, as EVO_VERBOSE=1 does; DryRun plans every
// change and runs no mutation; MaxConcurrency bounds executing callbacks (zero
// means GOMAXPROCS); State says where recorded status lives; Facades replaces the
// system boundaries, which is how a test runs Evo without a terminal.
type Config struct {
	Name string
	// Title is the 1.x subject shown in the conclusion.
	title   string
	Verbose bool
	Output  Output
	State   State
	Facades Facades

	// Subject is an optional durable line rendered once under the title (a repo path, a target host).
	// If unknown until the run has started, call Run.subject(text) instead.
	subject string

	// Facts are run-scoped name/value lines painted with the header, before any Task. Discovery
	// during work belongs on the Task that learned it (task.Fact).
	facts []factRecord

	// Stdout is the ordinary human stream (default os.Stdout); in FormatData it carries only domain payload.
	stdout io.Writer
	// Stderr owns diagnostics by default (default os.Stderr).
	stderr io.Writer
	// Result is an optional domain-payload writer; nil with FormatData means Stdout.
	result io.Writer
	// Stdin is the facade Confirm reads one answer line from (default os.Stdin).
	stdin io.Reader

	// Verbosity gates Verbose() print messages (default VerbosityNormal).
	verbosity verbosity
	// Color policy (default ColorAuto).
	color colorMode
	// Format is the projection mode (default FormatHuman). It routes streams only; encoding is Projection.
	format format
	// Projection selects presentation encoding. Zero defers to EVO_OUTPUT, else TTY/plain inference.
	projection projection

	// Debug is the lowest slog.Level the debug journal keeps (zero is LevelInfo; EVO_DEBUG may change it).
	Debug slog.Level
	debug debugConfig

	// Advanced (optional) — zero values inherit safe defaults.
	clock    timeSource
	redactor Redactor
	terminal terminalDriver
	strict   bool
	width    int
	// VisibilityDelay is the wait before the first live paint; nil means 80ms, non-nil is exact. Set with evo.Delay(d).
	visibilityDelay *time.Duration
	maxFrameRate    int
	maxEntities     int
	maxEvents       int
	// Plain disables live interactive frames, on a TTY or off.
	plain bool

	// Glyphs selects the state-glyph vocabulary (default GlyphsAuto).
	glyphs glyphProfile

	// FailedExitCode is the exit code when the conclusion is failed (zero means ExitFailed 2).
	failedExitCode int

	// DryRun declares the whole run a dry run: evo.Effect, evo.File, and evo.Exec render as [planned] rows.
	DryRun bool

	// Preview declares a preview before a confirm gate: DryRun's planned tense (Effect callbacks never run),
	// announced with Config.Subject alone instead of "[dry-run] <subject>". Use DryRun for a real --dry-run.
	preview bool

	// MaxConcurrency is the scheduler ceiling for executing callbacks (zero means GOMAXPROCS).
	MaxConcurrency int

	// StateDir, when non-empty, is the exact manifest state directory (<StateDir>/manifest-v1.json),
	// bypassing the default cache derivation (spec §11.3). Tests set it to t.TempDir().
	stateDir string

	// AppID overrides the application id in the default manifest cache path; ignored when StateDir is set.
	appID string

	// ProcessRunner overrides the facade evo.Exec spawns children through (spec §8.4); nil is the real OS runner.
	processRunner processRunner

	// FileFS overrides the facade evo.File does filesystem I/O through (spec §8.2); nil is the real filesystem.
	fileFS fs.FS
}

// Delay returns a non-nil *time.Duration for Config fields where zero is meaningful.
func delay(d time.Duration) *time.Duration {
	return &d
}

func defaultConfig() Config {
	return Config{
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

func resolveConfig(c Config) Config {
	base := defaultConfig()
	c.stdout = cmp.Or(c.stdout, c.Facades.Stdout, terminal.Stdout())
	c.stderr = cmp.Or(c.stderr, c.Facades.Stderr, terminal.Stderr())
	c.stdin = cmp.Or(c.stdin, c.Facades.Stdin, terminal.Stdin())
	c.clock = cmp.Or(c.clock, timeSource(c.Facades.Clock), timeSource(systemClock{}))
	c.terminal = cmp.Or(c.terminal, terminalDriver(c.Facades.Terminal))
	c.redactor = cmp.Or(c.redactor, c.Facades.Redactor, Redactor(process.NoopRedactor{}))
	c.processRunner = cmp.Or(c.processRunner, processRunner(c.Facades.Process), process.System())
	c.fileFS = cmp.Or(c.fileFS, fs.FS(c.Facades.FS), fs.FS(fs.Disk{}))
	c.stateDir = cmp.Or(c.stateDir, c.State.Dir)
	c.appID = cmp.Or(c.appID, c.State.AppID)
	c.title = cmp.Or(c.title, c.Name)
	if c.Verbose {
		c.verbosity = verbosityVerbose
	}
	switch c.Output {
	case Plain:
		c.plain, c.projection = true, projectionPlain
	case JSON:
		c.format, c.plain = formatJSON, true
	case JSONL:
		c.format, c.plain = formatJSONL, true
	}
	// LevelUnset (zero) takes Config.Debug's level; LevelTrace is non-zero.
	if c.debug.level == levelUnset {
		c.debug.level = logLevelOf(c.Debug)
	}
	// nil = unspecified; non-nil, 0 included, is intentional.
	if c.visibilityDelay == nil {
		c.visibilityDelay = base.visibilityDelay
	}
	c.width = cmp.Or(max(c.width, 0), base.width)
	c.maxFrameRate = cmp.Or(max(c.maxFrameRate, 0), base.maxFrameRate)
	c.maxEntities = cmp.Or(max(c.maxEntities, 0), base.maxEntities)
	c.maxEvents = cmp.Or(max(c.maxEvents, 0), base.maxEvents)
	return c
}

func newFromConfig(c Config) *Run { return newRun(configOf(c)) }

// configOf resolves Config one concern at a time; a later concern may refine an earlier one.
func configOf(c Config) config {
	cfg := baseConfig(c.title)
	cfg.streams(c)
	cfg.color(c)
	cfg.terminalOf(c)
	cfg.runtime(c)
	cfg.debugOf(c.debug)
	cfg.modes(c)
	return cfg
}

// humanWriter is stderr when stdout carries a document, stdout otherwise.
func humanWriter(c Config) io.Writer {
	switch c.format {
	case formatData, formatJSON, formatJSONL:
		return c.stderr
	default:
		return c.stdout
	}
}

// streams routes data-command progress to stderr, a machine Format's document to stdout, human output to stdout otherwise; diagnostics always to stderr.
func (cfg *config) streams(c Config) {
	cfg.primary, cfg.diagnostic, cfg.result = c.stdout, c.stderr, c.result
	switch c.format {
	case formatData:
		cfg.primary = c.stderr
		if c.result == nil {
			cfg.result = c.stdout
		}
	case formatExternal:
		cfg.plain = true
	case formatJSON, formatJSONL:
		cfg.primary, cfg.result = c.stderr, nil
		cfg.wireFormat, cfg.wireStream = c.format, c.stdout
	}
}

// color is off when asked, or in auto mode when NO_COLOR is set or human output is not a terminal.
func (cfg *config) color(c Config) {
	switch c.color {
	case colorNever:
		cfg.noColor = true
	case colorAlways:
	default:
		cfg.noColor = lookupEnv(envKeyNoColor) != "" || !writerIsCharDevice(humanWriter(c))
	}
}

// terminalOf picks the caller's driver, a live ANSI driver over a terminal human output reaches, or none (plain).
func (cfg *config) terminalOf(c Config) {
	liveWriter := humanWriter(c)
	switch {
	case c.terminal != nil:
		cfg.callerTerminal(c)
	case !c.plain && rendersInline(c.format) && writerIsCharDevice(liveWriter):
		cfg.liveTerminal(c, liveWriter)
	default:
		cfg.plain = true
	}
	if c.plain || c.projection.forcesPlain() {
		cfg.plain = true
	}
	cfg.projection = c.projection
}

func rendersInline(f format) bool {
	switch f {
	case formatHuman, formatData, formatJSON, formatJSONL:
		return true
	default:
		return false
	}
}

// callerTerminal installs the caller's driver. A driver reporting its own sink either shares a configured
// stream (its conclusion must not be written there twice) or becomes the primary stream.
func (cfg *config) callerTerminal(c Config) {
	cfg.terminal = c.terminal
	sr, ok := c.terminal.(sinkReporter)
	if !ok {
		return
	}
	if sink := sr.Sink(); sink != nil {
		switch sink {
		case c.stdout, c.stderr:
			cfg.samePrimaryAsTerminal = true
		default:
			cfg.primary = sink
		}
	}
	cfg.diagnosticSharesTerminal = sameTerminalDevice(sr.Sink(), c.stderr)
	cfg.resultSharesTerminal = sameTerminalDevice(sr.Sink(), cfg.result)
}

const liveTerminalHeight = 24

// liveTerminal builds the ANSI driver over liveWriter, sized from the terminal unless Config.Width says otherwise.
func (cfg *config) liveTerminal(c Config, liveWriter io.Writer) {
	width, height := c.width, liveTerminalHeight
	if width <= 0 {
		width = defaultWidth
	}
	if tw, th, ok := terminal.WriterSize(liveWriter); ok {
		if c.width <= 0 || c.width == defaultWidth {
			width = tw
		}
		height = th
	}
	cfg.terminal = terminal.NewANSI(liveWriter,
		terminal.WithInteractive(true), terminal.WithSize(width, height), terminal.WithSizeWriter(liveWriter))
	cfg.width = width
	cfg.samePrimaryAsTerminal = true
	cfg.diagnosticSharesTerminal = sameTerminalDevice(liveWriter, c.stderr)
	cfg.resultSharesTerminal = sameTerminalDevice(liveWriter, cfg.result)
}

func (cfg *config) runtime(c Config) {
	cfg.stdin = c.stdin
	cfg.visibilityDelay = defaultVisibilityDelay
	if c.visibilityDelay != nil {
		cfg.visibilityDelay = *c.visibilityDelay
	}
	if c.redactor != nil {
		cfg.redactor = c.redactor
	}
	cfg.clock, cfg.width, cfg.maxFrameRate = c.clock, c.width, c.maxFrameRate
	cfg.maxEntities, cfg.maxEvents, cfg.maxConcurrency = c.maxEntities, c.maxEvents, c.MaxConcurrency
	cfg.stateDir, cfg.appID = c.stateDir, c.appID
	cfg.processRunner, cfg.fileFS = c.processRunner, c.fileFS
}

// debugOf configures the debug journal: durable history above the live region, or a rolling pane in it.
func (cfg *config) debugOf(d debugConfig) {
	cfg.debugLevel = d.level
	if d.level == levelUnset {
		cfg.debugLevel = levelInfo
	}
	cfg.debugAddSource = d.addSource
	if d.view != debugPresentationPane {
		cfg.debugPresentation = debugPresentationHistory
		return
	}
	cfg.debugPresentation = debugPresentationPane
	cfg.debugPane = txt.DebugPane{Height: txt.DefaultDebugPaneHeight, NewestFirst: true, PreserveOnBad: true}
	if d.paneHeight > 0 {
		cfg.debugPane.Height = d.paneHeight
	}
	if d.newestFirst != nil {
		cfg.debugPane.NewestFirst = *d.newestFirst
	}
	cfg.debugPane.PreserveAlways = d.preserveAlways
}

func (cfg *config) modes(c Config) {
	cfg.strict, cfg.verbosity, cfg.glyphs = c.strict, c.verbosity, c.glyphs
	if c.failedExitCode != 0 {
		cfg.failedExitCode = c.failedExitCode
	}
	if c.DryRun || c.preview {
		cfg.dryRun = true
		cfg.dryRunHeaderText = c.subject
	}
	cfg.preview = c.preview
}

// initRun is the sole Run constructor: it builds a Run from cfg and arms first paint.
// It fills still-zero Config fields
// from EVO_OUTPUT, EVO_VERBOSE, EVO_DEBUG, NO_COLOR, and FORCE_COLOR. Explicit Config wins over
// env; env wins over TTY.
func initRun(configs ...Config) *Run {
	return initResolved(applyEnv(resolveInitConfig(configs)))
}

// initHostRun is initRun for a host entry point (Execute, Main): the environment is applied first, then
// stdout is reserved for the payload.
func initHostRun(cfg Config) *Run {
	return initResolved(payloadOnStdout(applyEnv(cfg)))
}

func initResolved(cfg Config) *Run {
	resolved := resolveConfig(cfg)
	out := newFromConfig(resolved)
	return finishInit(out, cfg, resolved.facts)
}

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

func resolveInitConfig(configs []Config) Config {
	if len(configs) == 0 {
		return Config{}
	}
	return configs[0]
}

// Environment keys honored at run start when the matching Config field is unset.
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

// Projection selects presentation encoding, independent of Format. Zero defers to EVO_OUTPUT, else TTY inference.
type projection int

const (
	// ProjectionHuman is ordinary TTY/plain inference; non-zero so an explicit value beats EVO_OUTPUT.
	projectionHuman projection = iota + 1
	projectionPlain
)

func (p projection) forcesPlain() bool { return p == projectionPlain }

var (
	lookupEnvMu sync.RWMutex
	lookupEnvFn = process.Env
)

func lookupEnv(key string) string {
	lookupEnvMu.RLock()
	fn := lookupEnvFn
	lookupEnvMu.RUnlock()
	if fn == nil {
		return process.Env(key)
	}
	return fn(key)
}

func applyEnv(c Config) Config {
	if c.projection == 0 && c.Output == 0 {
		c = selectOutputEnv(c, lookupEnv(envKeyOutput))
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

// selectOutputEnv applies EVO_OUTPUT's value v. json and jsonl pick the evo.run document and evo.event stream
// when no Format was chosen; a caller-chosen Format keeps its own stdout, so they change nothing there.
func selectOutputEnv(c Config, v string) Config {
	switch strings.TrimSpace(v) {
	case envOutputHuman:
		c.projection = projectionHuman
	case envOutputPlain:
		c.projection = projectionPlain
	case envOutputJSON:
		if c.format == formatHuman {
			c.format = formatJSON
		}
	case envOutputJSONL:
		if c.format == formatHuman {
			c.format = formatJSONL
		}
	}
	return c
}

// forceColorRequested reports whether FORCE_COLOR asks for color; NO_COLOR wins, so it is checked first.
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

// resolveGlyphProfileLocked turns an auto profile into a concrete one once, at construction.
func resolveGlyphProfileLocked(cfg *config) {
	if cfg.glyphs != glyphsAuto {
		return
	}
	if !interactiveOutputLocked(cfg) || localeAdvertisesUTF8() {
		cfg.glyphs = glyphsUnicode
		return
	}
	cfg.glyphs = glyphsASCII
}

// interactiveOutputLocked reports whether the terminal is a real interactive surface; the ASCII fallback guards only the live status column.
func interactiveOutputLocked(cfg *config) bool {
	ls := liveSurfaceOf(*cfg)
	return ls != nil && ls.IsInteractive()
}

// liveSurfaceOf is the terminal a live region paints on, unless it cannot paint one or the run is plain.
func liveSurfaceOf(cfg config) live.Surface {
	if cfg.plain {
		return nil
	}
	return live.SurfaceOf(cfg.terminal)
}

// localeAdvertisesUTF8 checks LC_ALL, then LC_CTYPE, then LANG for a UTF-8 marker, read at construction.
func localeAdvertisesUTF8() bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := process.Env(name); v != "" {
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "utf8")
		}
	}
	// No locale set: assume UTF-8 rather than guess ASCII from absence.
	return true
}

// rendererError reports a renderer failure as a genuine Run failure (§32.2); nil stays nil.
func rendererError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", errRenderer, err)
}

// eventStream is where the evo.event stream writes: Config.Stdout in Format JSONL, nowhere otherwise (§32.1).
func (c *config) eventStream() io.Writer {
	if c.wireFormat != formatJSONL {
		return nil
	}
	return c.wireStream
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

// context reports the run's cancellation signal (cancelled on interrupt and close). A nil Run reports a never-cancelled context.
func (o *Run) context() context.Context {
	if o == nil {
		return context.Background()
	}
	return o.graph.Context()
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
	record.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
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
	progress   = record.Progress
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
	debugPane         txt.DebugPane
	redactor          Redactor
	maxEntities       int
	maxEvents         int
	maxConcurrency    int
	extraWriters      []io.Writer
	verbosity         verbosity
	stateDir          string
	appID             string
	processRunner     processRunner
	fileFS            fs.FS
	stdin             io.Reader
	failedExitCode    int
	// dryRun selects ledger tense: [planned]/imperative when true, [changed]/past when false.
	dryRun bool
	// preview marks the planned tense as a pre-confirm preview: the header is the caller's subject alone, not "[dry-run] <subject>".
	preview bool
	glyphs  glyphProfile
	// samePrimaryAsTerminal: the live Terminal driver wraps the same writer as primary. Finish skips the
	// CON-009 dual-stream write so the conclusion band is not rendered twice. Set only at construction.
	samePrimaryAsTerminal bool
	// diagnosticSharesTerminal: Diagnostics is the same physical terminal as the live driver (fd 1 and 2 on one tty).
	// projectDebugRecordLocked then clears live, writes durable, repaints, so raw bytes never land mid-spinner-row.
	diagnosticSharesTerminal bool
	// resultSharesTerminal: the payload stream is the same physical terminal as the live driver, so payload
	// text goes through the live region instead of landing inside it.
	resultSharesTerminal bool
	// dryRunHeaderText is Config.Subject's text so a dry-run's announcement merges onto one "[dry-run] <subject>"
	// line. Only the synchronous run-start path sets it; a later switch to dry run keeps the marker alone.
	dryRunHeaderText string
	// wireFormat is non-zero only for FormatJSON/FormatJSONL (spec §32.1). wireStream receives the v2
	// document/event lines (Config.Stdout); presentation still goes to primary/diagnostic.
	wireFormat format
	wireStream io.Writer
}

// LogLevel is a diagnostic severity. Zero is LevelUnset; named levels start at LevelTrace.
type logLevel int

const (
	levelUnset logLevel = iota
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

type terminalDriver = terminal.Driver

// sinkReporter is implemented by a TerminalDriver that knows its destination writer, so construction
// can detect a caller-supplied terminal writing to an already-known stream.
type sinkReporter interface {
	Sink() io.Writer
}

const (
	defaultVisibilityDelay = 80 * time.Millisecond
	defaultMaxFrameRate    = 20
	defaultWidth           = 80
	defaultMaxEntities     = 100_000
	defaultMaxEvents       = 50_000
)

type debugPresentation int

const (
	debugPresentationHistory debugPresentation = iota
	debugPresentationPane
)

func (o *Run) humanStyle() txt.Style {
	return txt.Style{Color: !o.cfg.noColor, Verbose: o.cfg.verbosity >= verbosityVerbose, Profile: o.cfg.glyphs}
}

func (c *config) debugPanePresentation() *txt.DebugPane {
	if c.debugPresentation != debugPresentationPane {
		return nil
	}
	return &c.debugPane
}

// runHost is the Run as graph, freshness and change borrow it: it implements graph.Host,
// freshness.Narration and change.Engine.
type runHost struct{ out *Run }

const resourceWaitingPrefix = "waiting for "

func (o *Run) newChangeHost() change.Host {
	return change.NewHost(change.HostParts{Engine: runHost{out: o}, Record: o.rec, Graph: o.graph, Freshness: o.freshness})
}

func (e runHost) Lock()   { e.out.mu.Lock() }
func (e runHost) Unlock() { e.out.mu.Unlock() }

// DryRun reports whether the Run plans instead of making changes; fixed at construction, so lock-free.
func (e runHost) DryRun() bool { return e.out.cfg.dryRun }

func (e runHost) FS() fs.FS {
	e.out.mu.Lock()
	defer e.out.mu.Unlock()
	return e.out.cfg.fileFS
}

// LedgerTense is the tense id's rows take, and the shared guard behind every change a Task records.
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

func (e runHost) CheckOpen() error {
	err := e.out.ensureOpen()
	if err != nil {
		e.out.recordMisuse(err)
	}
	return err
}

func (e runHost) RecordMisuse(err error) { e.out.recordMisuse(err) }

// NoteManifestWarning surfaces a safe cache-miss warning (spec §11.3) as a run-scoped Fact, never an error.
func (e runHost) NoteManifestWarning(warning error) { e.out.Fact("manifest", warning.Error()) }

// ShowWaiting makes id's row read "waiting for <label>" and returns the restore function.
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

func (e runHost) SectionOpened(_ record.TaskID, section string, tense record.LedgerTense) {
	e.out.bumpLocked()
	e.out.appendEventLocked(recEvent{Type: tense.DeclaredEvent(), EntityID: section})
}

// EntryRecorded journals a recorded ledger row and narrates it as the §38 effect line.
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

// VerificationRecorded files the Task's row again and narrates each detail as evo.run projects it.
func (e runHost) VerificationRecorded(task record.TaskID, details []record.VerificationDetail) {
	o := e.out
	o.show.Board.Changed(task)
	for _, d := range details {
		o.emitWireEventLocked(wire.EventVerificationObserved, string(task), wire.ToVerificationDoc(d).EventPayload())
	}
}

// stateOf is the render state of the graph's Task t. Callers hold o.mu.
func (o *Run) stateOf(t *graph.Task) *taskState { return o.taskStates[t.ID] }

func (runHost) Ending(err error) (record.State, record.Problem) { return endingOf(err) }

func (e runHost) Changed(*graph.Task) { e.out.followRecordLocked() }

// EndingName names the Task carrying the run's own ending: the subject, else the executable.
func (e runHost) EndingName() string { return e.out.endingName() }

// TaskDeclared registers a just-declared Task's row, live index, census place, events, and a forced paint.
func (e runHost) TaskDeclared(t *graph.Task) {
	o := e.out
	st := &taskState{id: t.ID, node: t, name: t.Name}
	st.handle = &Task{out: o, id: st.id, state: st}
	o.taskStates[st.id] = st
	o.appendTaskLocked(st, false)
}

// GateDeclared registers the gate of a container builder: no row, no collection, followed like any Task.
func (e runHost) GateDeclared(t *graph.Task) {
	o := e.out
	gate := &taskState{id: t.ID, node: t, name: t.Name}
	gate.handle = &Task{out: o, id: gate.id}
	o.taskStates[gate.id] = gate
	o.show.DeclareGate(t.TaskID(), t.Rec)
}

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
	st.handle = &Group{out: o, id: st.id}
	o.show.DeclareGroup(txt.DeclaredGroup{
		ID: c.ID, Name: c.Name, Declaration: c.Declaration, Sequential: c.Sequential, Rec: c.Rec, Node: c,
	}, parentID)
}

// Started is the render work a claimed Task owes (eligibility event, Running, snapshot version, forced
// paint). It reports false when an interrupt settled the Task first, so the scheduler skips its callback.
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
		// An interrupt settled the Task between its claim and this step: never started.
		return false
	}
	o.emitWireEventLocked(wire.EventTaskEligible, st.id, nil)
	if t.Rec.State() == pending {
		o.promoteRunningLocked(st)
	} else {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.show.Live.Signal(true)
	return true
}

func (e runHost) Resolving(t *graph.Task) { e.out.show.Live.HoldRunning(record.TaskID(t.ID)) }

// AlreadyResolved records a second terminal verb on t as misuse, keeping the rejected summary.
func (e runHost) AlreadyResolved(t *graph.Task, summary string) {
	e.out.recordAlreadyResolvedLocked(t.Name, summary)
}

// Disowned disowns every Effect in flight for t: an Effect whose callback resolved its own Task as
// anything but Done and returned nil handled the failure itself, so its work must not reach the ledger.
func (e runHost) Disowned(t *graph.Task) {
	if id := t.TaskID(); e.out.changes.EffectInFlight(id) {
		e.out.changes.DenyEffect(id)
	}
}

// Settled writes the settled Task where readers find it. Only a Done Task commits its manifest record
// (spec §8.2/§11.3).
func (e runHost) Settled(t *graph.Task) {
	st := e.out.fileSettledLocked(t)
	if st != nil && t.Rec.State().Succeeded() {
		e.out.commitManifestLocked(st)
	}
}

// Overturned writes the Task's second settle, Failed after Done, where readers find it, and takes back the
// manifest record its first settle committed: the next Run must not skip work this Run found to conflict.
func (e runHost) Overturned(t *graph.Task) {
	if st := e.out.fileSettledLocked(t); st != nil {
		e.out.freshness.Retract(t, freshness.ModeFor(e.out.cfg.dryRun))
	}
}

// fileSettledLocked writes the settled Task where readers find it and returns its state, nil when it has none.
// A Group or Sequence child repaints the live ledger; a standalone Task commits its rows to scrollback at once so
// later Printf/Confirm output never lands above finished work. Callers hold o.mu.
func (o *Run) fileSettledLocked(t *graph.Task) *taskState {
	o.followRecordLocked()
	st := o.stateOf(t)
	if st == nil {
		return nil
	}
	if st.collection() != nil {
		o.show.Live.Signal(true)
	} else {
		o.show.Stream.Commit(record.TaskID(st.id))
		o.show.Stream.CommitEffects(st.id)
	}
	return st
}

// commitManifestLocked persists st's operations once it settled Done and narrates the commit. Callers hold o.mu.
func (o *Run) commitManifestLocked(st *taskState) {
	commit := o.freshness.Commit(o.graph.Context(), st.node, freshness.ModeFor(o.cfg.dryRun))
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

// RunEnded registers the synthetic Task carrying the run's own ending, already settled.
func (e runHost) RunEnded(t *graph.Task) {
	o := e.out
	o.appendTaskLocked(&taskState{id: t.ID, node: t, name: t.Name}, true)
}

// narrate streams one §38 event about a Task under the Run's lock; a run that streams no events skips the lock.
func (e runHost) narrate(event, entityID string, payload func() map[string]any) {
	o := e.out
	if !o.show.Events.On() {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(event, entityID, payload())
}

func noPayload() map[string]any { return nil }

func (e runHost) DefinitionStarted(t *graph.Task) {
	e.narrate(wire.EventDefinitionStarted, t.ID, noPayload)
}

func (e runHost) DefinitionFinished(t *graph.Task, failed bool) {
	e.narrate(wire.EventDefinitionFinished, t.ID, func() map[string]any {
		return map[string]any{"failed": failed}
	})
}

func (e runHost) EvidenceEvaluated(t *graph.Task, phase graph.EvidencePhase, evidence record.EvidencePhase) {
	e.narrate(wire.EventEvidenceEvaluated, t.ID, func() map[string]any {
		name := "before_definition"
		if phase == graph.EvidenceAfter {
			name = "after_definition"
		}
		return map[string]any{
			"phase":     name,
			"evaluated": evidence.Evaluated,
			"satisfied": evidence.Satisfied,
			"source":    evidence.Source,
		}
	})
}

func (e runHost) BasisJudged(id record.TaskID, judgement freshness.BasisJudgement) {
	e.narrate(wire.EventBasisFingerprinted, string(id), func() map[string]any {
		return map[string]any{"count": len(judgement.Observed), "current": judgement.Current}
	})
}

// VerifierObserved narrates one Verify check's outcome (spec §38), named by registration order.
func (e runHost) VerifierObserved(id record.TaskID, index int, outcome freshness.VerifierOutcome) {
	e.narrate(wire.EventVerificationObserved, string(id), func() map[string]any {
		status := wire.VerificationUnsatisfied
		switch outcome {
		case freshness.VerifierErrored:
			status = wire.VerificationError
		case freshness.VerifierSatisfied:
			status = wire.VerificationSatisfied
		}
		return wire.VerificationDoc{Name: fmt.Sprintf("verify_%d", index), Status: status}.EventPayload()
	})
}

// Observe narrates one step of a tracked operation as a §38 "evo.event" line; the wire vocabulary stays at this edge.
func (e runHost) Observe(ob change.Observation) {
	e.out.emitWireEventLocked(observationEvent(ob.Kind), string(ob.Task), observationPayload(ob))
}

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

// observationPayload is the §38 payload for ob: the subject's kind and name under its kind's
// historical key, then the step's fields. A File's basis.fingerprinted line never names its kind.
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

// Action is a recommended next step: argv a machine can run, or text only a human can act on.
type Action struct {
	Command              []string
	Label                string
	uRL                  string
	file                 string
	explanation          string
	requiresConfirmation bool
	destructive          bool
}

func argvOf(cmd *commandSpec) []string {
	if cmd == nil {
		return nil
	}
	return append([]string{cmd.Executable}, cmd.Args...)
}

func commandOf(argv []string) *commandSpec {
	if len(argv) == 0 {
		return nil
	}
	return &commandSpec{Executable: argv[0], Args: slices.Clone(argv[1:])}
}
