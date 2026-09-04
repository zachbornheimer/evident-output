package evo

import (
	"io"
	"time"
)

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
	noColor           bool
	width             int
	clock             TimeSource
	visibilityDelay   time.Duration
	maxFrameRate      int
	strict            bool
	terminal          TerminalDriver
	debugLevel        LogLevel
	debugPresentation DebugPresentation
	debugPane         debugPaneConfig
	redactor          Redactor
	projection        ProjectionPolicy
	maxEntities       int
	maxEvents         int
	extraWriters      []io.Writer
	verbosity         Verbosity
	// stdin is the facade Confirm reads one answer line from (default os.Stdin,
	// resolved lazily so NewWithOptions callers that skip Stdin still work).
	stdin io.Reader
	// failedExitCode overrides ExitFailed when conclusion is StateFailed.
	// Zero means use ExitFailed (2).
	failedExitCode int
	// dryRun selects mutation-verb tense: true renders TaskHandle mutation
	// verbs as [planned]/imperative, false as [changed]/past tense.
	dryRun bool
	// glyphs selects the state-glyph vocabulary (GlyphsAuto resolved at
	// construction to GlyphsUnicode or GlyphsASCII; see glyph.go).
	glyphs GlyphProfile
	// samePrimaryAsTerminal records that the live Terminal driver was built
	// around the same underlying writer as primary (Config's default-construction
	// path: To(c.Stdout) and Terminal(ansi-over-c.Stdout) target one stream).
	// Set only where construction knows both writers, never inferred later by
	// comparing file descriptors — see configToOptions. Finish uses it to skip
	// the CON-009 dual-stream write, which would otherwise render the
	// conclusion band twice on the one physical screen.
	samePrimaryAsTerminal bool
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// To sets the primary human writer.
func To(w io.Writer) Option {
	return optionFunc(func(c *config) { c.primary = w })
}

// Diagnostics sets the diagnostic writer for Debug history and Capture mirrors.
// When set and distinct from the primary writer (To), Debug lines are not also
// written to the human primary stream — use dual-stream for LaunchAgent /
// data-command layouts (human on stdout, diagnostics on stderr).
func Diagnostics(w io.Writer) Option {
	return optionFunc(func(c *config) { c.diagnostic = w })
}

// ResultStream sets the domain-payload writer (see Output.ResultWriter).
// Presentation never writes here. FormatData defaults this to Config.Stdout.
func ResultStream(w io.Writer) Option {
	return optionFunc(func(c *config) {
		if w != nil {
			c.result = w
		}
	})
}

// Plain forces final-report projection (no live spinner region).
// Semantic color is still emitted unless NoColor is set.
func Plain() Option {
	return optionFunc(func(c *config) { c.plain = true })
}

// NoColor disables color.
func NoColor() Option {
	return optionFunc(func(c *config) { c.noColor = true })
}

// Width sets the terminal width in columns.
func Width(columns int) Option {
	return optionFunc(func(c *config) { c.width = columns })
}

// Clock injects a time source facade.
func Clock(ts TimeSource) Option {
	return optionFunc(func(c *config) { c.clock = ts })
}

// VisibilityDelay sets how long live activity must persist before the first
// interactive paint (default 80ms). Zero paints immediately. Prevents Phase→fast
// Done spinner flash (H.2). Domain TimeSource is used for the threshold.
func VisibilityDelay(delay time.Duration) Option {
	return optionFunc(func(c *config) { c.visibilityDelay = delay })
}

// MaxFrameRate caps interactive redraws per second.
func MaxFrameRate(framesPerSecond int) Option {
	return optionFunc(func(c *config) { c.maxFrameRate = framesPerSecond })
}

// Strict enables panic-on-misuse for tests.
func Strict() Option {
	return optionFunc(func(c *config) { c.strict = true })
}

// DryRun declares this run a dry run: TaskHandle mutation verbs (Delete,
// Create, Update, Remove, Write, Push, Record, RecordName) render as
// [planned] rows with imperative verbs instead of [changed] rows with
// past-tense verbs. Set once via Config.DryRun in ordinary application code;
// this Option exists for the advanced NewWithOptions surface and tests.
func DryRun() Option {
	return optionFunc(func(c *config) { c.dryRun = true })
}

// Stdin injects the reader Confirm reads answers from (facade rule — no
// direct os.Stdin read in Confirm's logic). Default os.Stdin.
func Stdin(r io.Reader) Option {
	return optionFunc(func(c *config) { c.stdin = r })
}

// Terminal injects a terminal driver (interactive projection; v0.2).
func Terminal(driver TerminalDriver) Option {
	return optionFunc(func(c *config) { c.terminal = driver })
}

// withPrimarySharesTerminal marks that the just-configured Terminal driver
// writes to the same stream as primary (To). Only configToOptions calls this
// — it knows both writers at construction time, so Finish never has to guess
// stream identity from an fd comparison.
func withPrimarySharesTerminal() Option {
	return optionFunc(func(c *config) { c.samePrimaryAsTerminal = true })
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

// Debug is the debug log level (Appendix H). Alias of LevelDebug.
const Debug = LevelDebug

// DebugLevel sets the minimum debug emission level.
// Pass LevelTrace or LevelDebug to surface Debug journal lines.
func DebugLevel(level LogLevel) Option {
	return optionFunc(func(c *config) {
		if level == LevelUnset {
			c.debugLevel = LevelInfo
			return
		}
		c.debugLevel = level
	})
}

// TerminalDriver is the exclusive owner of terminal control sequences.
// Interactive implementation arrives in v0.2; the interface is defined early
// so options and tests compile.
type TerminalDriver interface {
	ID() string
}

// sinkReporter is implemented by a TerminalDriver that knows its own
// destination writer (terminal.ANSI, testkit's drivers). configToOptions
// uses it to DETECT whether a caller-supplied Terminal(...) happens to write
// to the same stream as primary (To), instead of only knowing that for the
// one construction path that builds both itself — see terminalSharesPrimary.
type sinkReporter interface {
	Sink() io.Writer
}

// terminalSharesPrimary reports whether driver's own writer, if it exposes
// one via sinkReporter, is the same stream as primary. A driver that doesn't
// implement sinkReporter (or has no fixed sink, e.g. a virtual test screen)
// answers false — that's the safe default (dual-write), never a
// false-positive skip of the conclusion band.
func terminalSharesPrimary(driver TerminalDriver, primary io.Writer) bool {
	if driver == nil || primary == nil {
		return false
	}
	sr, ok := driver.(sinkReporter)
	if !ok {
		return false
	}
	return sr.Sink() == primary
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
func MaxEntities(n int) Option {
	return optionFunc(func(c *config) { c.maxEntities = n })
}

// MaxEvents caps durable journal events; when exceeded, oldest non-critical
// events are dropped so critical terminal events are retained (CON-008).
func MaxEvents(n int) Option {
	return optionFunc(func(c *config) { c.maxEvents = n })
}

// AlsoWrite adds an additional human projection writer. On Finish, each writer
// receives the plain projection; failures on one do not skip the others (CON-009).
func AlsoWrite(w io.Writer) Option {
	return optionFunc(func(c *config) {
		if w != nil {
			c.extraWriters = append(c.extraWriters, w)
		}
	})
}
