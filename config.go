package evo

import (
	"cmp"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/fs"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	"github.com/zachbornheimer/evident-output/v2/internal/project/live"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// Config holds a run's settings, passed to Main or Execute. The zero Config is
// complete: Name defaults to the binary name, and Output to Human on a terminal
// and Plain otherwise. Name titles the run and is the name in the evo.run
// document; Verbose shows hidden Facts, as EVO_VERBOSE=1 does; DryRun plans every
// change and runs no mutation; Debug is the lowest level the debug journal keeps, and nil keeps it off unless
// EVO_DEBUG names a level;
// MaxConcurrency bounds executing callbacks (zero means GOMAXPROCS); State says
// where recorded status lives; Facades replaces the system boundaries, which is
// how a test runs Evo without a terminal.
type Config struct {
	Name           string
	DryRun         bool
	Verbose        bool
	Output         Output
	Debug          slog.Leveler
	MaxConcurrency int
	State          State
	Facades        Facades
}

// State is where recorded status lives: Dir is the directory (zero means the
// platform state directory) and AppID its namespace (zero means the binary name).
type State struct {
	Dir   string
	AppID string
}

// tuning is what a Run is built with beyond Config. Production passes the zero
// tuning, so every field is a test seam the contract's Facades cannot express;
// TestTuningSeamsAreNamed pins the set.
type tuning struct {
	// color pins colour on or off, so a test is independent of NO_COLOR and FORCE_COLOR.
	color colorMode
	// width is the layout width of a non-terminal run; a narrow-layout test sets it, since a buffer has no size.
	width int
	// visibilityDelay is the wait before the first live paint; nil means 80ms, non-nil is exact.
	// A live-region test sets it to zero so its first frame paints at once.
	visibilityDelay *time.Duration
	// maxFrameRate bounds live repaints per second; a scenario test lifts it so every frame paints.
	maxFrameRate int
	// maxEntities is the declaration cap; a limit test lowers it instead of declaring 100,000 Tasks.
	maxEntities int
	// maxEvents is the journal cap; a limit test lowers it instead of emitting 50,000 events.
	maxEvents int
}

type colorMode int

const (
	colorAuto colorMode = iota
	colorAlways
	colorNever
)

// logLevel is a diagnostic severity. Zero is levelUnset; named levels start at levelTrace; levelOff keeps
// the journal closed to every record.
type logLevel int

const (
	levelUnset logLevel = iota
	levelTrace
	levelDebug
	levelInfo
	levelWarn
	levelError
	levelOff
)

type terminalDriver = terminal.Driver

// sinkReporter is implemented by a terminal driver that knows its destination writer, so construction
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
	liveTerminalHeight     = 24
)

// config is Config and tuning resolved once, with the environment applied: what a Run reads.
type config struct {
	subject string
	// primary carries rows and free text; result carries the Human and Plain payload.
	primary         io.Writer
	diagnostic      io.Writer
	result          io.Writer
	extraWriters    []io.Writer
	plain           bool
	noColor         bool
	width           int
	clock           timeSource
	visibilityDelay time.Duration
	maxFrameRate    int
	terminal        terminalDriver
	debugLevel      logLevel
	redactor        Redactor
	maxEntities     int
	maxEvents       int
	maxConcurrency  int
	// verbose shows Facts and expands each Skipped/Kept row into one line per reason.
	verbose       bool
	stateDir      string
	appID         string
	processRunner processRunner
	fileFS        fs.FS
	stdin         io.Reader
	// dryRun selects ledger tense: [planned]/imperative when true, [changed]/past when false.
	dryRun bool
	glyphs glyphProfile
	// samePrimaryAsTerminal: the terminal driver wraps the primary writer, so Finish does not write
	// the conclusion there a second time.
	samePrimaryAsTerminal bool
	// diagnosticSharesTerminal: the diagnostic stream is the live driver's terminal, so a debug
	// record clears the live region, writes, and repaints.
	diagnosticSharesTerminal bool
	// resultSharesTerminal: the payload stream is the live driver's terminal, so payload text goes through
	// the live region instead of landing inside it.
	resultSharesTerminal bool
	// wireFormat is JSON or JSONL for a machine Output, zero otherwise; wireStream is stdout.
	wireFormat Output
	wireStream io.Writer
}

// resolveConfig is the one place Config becomes config. Explicit settings win over the environment
// (EVO_OUTPUT, EVO_VERBOSE, EVO_DEBUG, NO_COLOR, FORCE_COLOR), and the environment over terminal inference.
// Rows and free text go to stderr; stdout carries only the payload: Run.ResultWriter's bytes in Human and
// Plain, the evo.run document in JSON, the evo.event stream in JSONL.
func resolveConfig(c Config, t tuning) config {
	f := c.Facades
	stdout := cmp.Or(f.Stdout, terminal.Stdout())
	stderr := cmp.Or(f.Stderr, terminal.Stderr())
	output := cmp.Or(c.Output, outputFromEnv(lookupEnv(envKeyOutput)))
	cfg := config{
		subject:         cmp.Or(c.Name, identityFallbackName()),
		primary:         stderr,
		diagnostic:      stderr,
		plain:           output == Plain,
		width:           cmp.Or(max(t.width, 0), defaultWidth),
		clock:           cmp.Or(timeSource(f.Clock), timeSource(systemClock{})),
		visibilityDelay: defaultVisibilityDelay,
		maxFrameRate:    cmp.Or(max(t.maxFrameRate, 0), defaultMaxFrameRate),
		redactor:        cmp.Or(f.Redactor, Redactor(process.NoopRedactor{})),
		maxEntities:     cmp.Or(max(t.maxEntities, 0), defaultMaxEntities),
		maxEvents:       cmp.Or(max(t.maxEvents, 0), defaultMaxEvents),
		maxConcurrency:  c.MaxConcurrency,
		stateDir:        c.State.Dir,
		appID:           c.State.AppID,
		processRunner:   cmp.Or(processRunner(f.Process), process.System()),
		fileFS:          cmp.Or(fs.FS(f.FS), fs.FS(fs.Disk{})),
		stdin:           cmp.Or(f.Stdin, terminal.Stdin()),
		dryRun:          c.DryRun,
	}
	if output == JSON || output == JSONL {
		cfg.wireFormat, cfg.wireStream = output, stdout
	} else {
		cfg.result = stdout
	}
	if t.visibilityDelay != nil {
		cfg.visibilityDelay = *t.visibilityDelay
	}
	if c.Verbose || lookupEnv(envKeyVerbose) == envVerboseOn {
		cfg.verbose = true
	}
	cfg.colorOf(t.color, stderr)
	cfg.terminalOf(f.Terminal, stdout, stderr)
	cfg.debugOf(c.Debug)
	return cfg
}

// colorOf is off when asked, or in auto mode when NO_COLOR is set or human output is not a terminal;
// FORCE_COLOR turns auto mode on.
func (cfg *config) colorOf(mode colorMode, human io.Writer) {
	if mode == colorAuto && forceColorRequested() {
		mode = colorAlways
	}
	switch mode {
	case colorNever:
		cfg.noColor = true
	case colorAlways:
	default:
		cfg.noColor = lookupEnv(envKeyNoColor) != "" || !writerIsCharDevice(human)
	}
}

// terminalOf picks the caller's driver, a live ANSI driver over a terminal stderr reaches, or none (plain).
func (cfg *config) terminalOf(driver Terminal, stdout, stderr io.Writer) {
	switch {
	case driver != nil:
		cfg.callerTerminal(driver, stdout, stderr)
	case !cfg.plain && writerIsCharDevice(stderr):
		cfg.liveTerminal(stderr)
	default:
		cfg.plain = true
	}
}

// callerTerminal installs the caller's driver. A driver reporting its own sink either shares a configured
// stream or becomes the primary one; either way it renders the conclusion there, so Finish does not again.
func (cfg *config) callerTerminal(driver Terminal, stdout, stderr io.Writer) {
	cfg.terminal = driver
	sr, ok := driver.(sinkReporter)
	if !ok {
		return
	}
	if sink := sr.Sink(); sink != nil {
		if sink != stdout && sink != stderr {
			cfg.primary = sink
		}
		cfg.samePrimaryAsTerminal = true
	}
	cfg.diagnosticSharesTerminal = sameTerminalDevice(sr.Sink(), stderr)
	cfg.resultSharesTerminal = sameTerminalDevice(sr.Sink(), cfg.result)
}

// liveTerminal builds the ANSI driver over w, sized from the terminal unless tuning set a width.
func (cfg *config) liveTerminal(w io.Writer) {
	width, height := cfg.width, liveTerminalHeight
	if tw, th, ok := terminal.WriterSize(w); ok {
		if cfg.width == defaultWidth {
			width = tw
		}
		height = th
	}
	cfg.terminal = terminal.NewANSI(w,
		terminal.WithInteractive(true), terminal.WithSize(width, height), terminal.WithSizeWriter(w))
	cfg.width = width
	cfg.samePrimaryAsTerminal = true
	cfg.diagnosticSharesTerminal = true
	cfg.resultSharesTerminal = sameTerminalDevice(w, cfg.result)
}

// debugOf sets the journal's level: Config.Debug, else EVO_DEBUG, else off.
func (cfg *config) debugOf(level slog.Leveler) {
	if level != nil {
		cfg.debugLevel = logLevelOf(level.Level())
		return
	}
	cfg.debugLevel, _ = parseDebugEnv(lookupEnv(envKeyDebug))
	if cfg.debugLevel == levelUnset {
		cfg.debugLevel = levelOff
	}
}

// eventStream is where the evo.event stream writes: stdout in JSONL, nowhere otherwise.
func (c *config) eventStream() io.Writer {
	if c.wireFormat != JSONL {
		return nil
	}
	return c.wireStream
}

// resolveGlyphProfileLocked picks the glyph profile once, at construction.
func resolveGlyphProfileLocked(cfg *config) {
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
	for _, v := range []string{process.Env(envKeyLocaleAll), process.Env(envKeyLocaleType), process.Env(envKeyLang)} {
		if v != "" {
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "utf8")
		}
	}
	// No locale set: assume UTF-8 rather than guess ASCII from absence.
	return true
}
