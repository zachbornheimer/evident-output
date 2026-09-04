package evo

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/zachbornheimer/evident-output/terminal"
)

// processArgv0 is the facade over os.Args[0] (facade rule: no direct
// os.Args read anywhere else) — a var so tests can inject a fixed value
// instead of depending on the real test binary's path.
var processArgv0 = func() string {
	if len(os.Args) == 0 {
		return ""
	}
	return os.Args[0]
}

// identityFallbackName is the executable's own basename, used only when an
// output-level outcome (Output.Failf/Cancel) has no named task and no
// explicit Config.Title to identify it with — replacing the generic literal
// "command" with the caller's actual binary name (I2). This is deliberately
// NOT plumbed into Snapshot.Subject / the conclusion band's Subject: that
// text is dialect-frozen this release and many existing goldens depend on
// its "no Subject configured" fallback (bare state name) staying exactly as
// it is — Config.Title stays the only way to set that.
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
)

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
	// Output.Subject(text) instead — same one-shot durable-line semantics,
	// as a post-construction setter (I3).
	Subject string

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
	// Format projection mode (default FormatHuman).
	Format Format

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
	// TaskHandle mutation verb (Delete, Create, Update, Remove, Write, Push,
	// Record, RecordName) renders as a [planned] row with the imperative verb
	// instead of a [changed] row with the past-tense verb. No call site writes
	// its own tense.
	DryRun bool

	// Isolated returns an independent Output that never touches package
	// state: it is not installed as the package-level default and does not
	// arm first paint. Use for parallel tests and embedders that hold their
	// own *Output instead of going through Default()/Task()/Print() et al.
	// Not consulted when Options is set: that path already never installs
	// the default or arms first paint, regardless of Isolated's value —
	// see Options.
	Isolated bool

	// Options is the advanced, raw Option escape hatch for tests and
	// specialized embedding (custom Terminal, Clock, exact writer wiring)
	// that need to bypass Config's ordinary stream/TTY/color inference
	// entirely. When set, every other Config field except Title, DryRun, and
	// Subject is ignored and the Output is built from these Options alone.
	// Isolated is not honored differently here: this path never installs
	// the result as the package-level default or arms first paint, exactly
	// as if Isolated were always true — see Init.
	Options []Option
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
		c.Stdout = os.Stdout
	}
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}
	if c.Stdin == nil {
		c.Stdin = os.Stdin
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
		c.Clock = SystemClock{}
	}
	if c.Redactor == nil {
		c.Redactor = NoopRedactor{}
	}
	return c
}

func newFromConfig(c Config) *Output {
	opts := configToOptions(c)
	return newOutput(c.Title, opts...)
}

func configToOptions(c Config) []Option {
	var opts []Option

	// primaryWriter mirrors whichever writer To() receives below — kept as a
	// value (not re-derived) so the Terminal-sharing detection further down
	// compares against the exact same stream, per format.
	primaryWriter := c.Stdout

	// Stream routing
	switch c.Format {
	case FormatData:
		// Human presentation on stderr; domain payload on Result (default Stdout).
		primaryWriter = c.Stderr
		opts = append(opts, To(c.Stderr), Diagnostics(c.Stderr), DataProjection())
		resultW := c.Result
		if resultW == nil {
			resultW = c.Stdout
		}
		opts = append(opts, ResultStream(resultW))
	case FormatExternal:
		opts = append(opts, To(c.Stdout), Diagnostics(c.Stderr), ExternalProjection())
		if c.Result != nil {
			opts = append(opts, ResultStream(c.Result))
		}
	default:
		opts = append(opts, To(c.Stdout), Diagnostics(c.Stderr))
		if c.Result != nil {
			opts = append(opts, ResultStream(c.Result))
		}
	}

	// Color / TTY
	noColor := false
	switch c.Color {
	case ColorNever:
		noColor = true
	case ColorAlways:
		// keep color
	default: // ColorAuto
		if os.Getenv("NO_COLOR") != "" {
			noColor = true
		}
		if !IsCharDevice(c.Stdout) && c.Format != FormatData {
			// Off-TTY human primary: no CSI.
			noColor = true
		}
		if c.Format == FormatData && !IsCharDevice(c.Stderr) {
			noColor = true
		}
	}
	if noColor {
		opts = append(opts, NoColor())
	}

	// Interactive live region only on a real TTY and human format.
	wantLive := !c.Plain && c.Format == FormatHuman
	liveWriter := c.Stdout
	if c.Format == FormatData {
		liveWriter = c.Stderr
		wantLive = !c.Plain && IsCharDevice(c.Stderr)
	} else {
		wantLive = wantLive && IsCharDevice(c.Stdout)
	}
	switch {
	case c.Terminal != nil:
		opts = append(opts, Terminal(c.Terminal))
		// A caller-supplied driver (Config.Terminal or the Options path) may
		// still write to the same physical stream as primary — DETECT that
		// via the driver's own Sink() rather than requiring the caller to
		// say so, closing the examples/terminal-driver double-band gap (X3).
		if terminalSharesPrimary(c.Terminal, primaryWriter) {
			opts = append(opts, withPrimarySharesTerminal())
		}
	case wantLive:
		width, height := c.Width, 24
		if width <= 0 {
			width = defaultWidth
		}
		// Prefer real terminal dimensions when liveWriter is a TTY *os.File.
		if f, ok := liveWriter.(*os.File); ok {
			if tw, th, ok := terminal.Size(f); ok {
				// Caller Width>0 is a deterministic override; otherwise use real cols.
				if c.Width <= 0 || c.Width == defaultWidth {
					width = tw
				}
				height = th
			} else {
				// TTY without ioctl size (pty, ssh, `timeout`): keep live
				// with default geometry so a spinner still appears.
				height = 24
			}
		}
		if wantLive {
			ansiOpts := []terminal.Option{
				terminal.WithInteractive(true),
				terminal.WithSize(width, height),
			}
			// Re-query geometry on each live redraw (resize-aware path).
			if f, ok := liveWriter.(*os.File); ok {
				ansiOpts = append(ansiOpts, terminal.WithSizeFile(f))
			}
			opts = append(opts, Terminal(terminal.NewANSI(liveWriter, ansiOpts...)))
			opts = append(opts, Width(width))
			// liveWriter is the same stream To() was already given above
			// (c.Stdout, or c.Stderr in FormatData) — the terminal and
			// primary are one physical destination, so Finish must not
			// dual-write the conclusion band a second time.
			opts = append(opts, withPrimarySharesTerminal())
		} else {
			opts = append(opts, Plain())
		}
	default:
		opts = append(opts, Plain())
	}
	if c.Plain {
		opts = append(opts, Plain())
	}

	if c.Stdin != nil {
		opts = append(opts, Stdin(c.Stdin))
	}
	opts = append(opts, Clock(c.Clock), Redact(c.Redactor), Width(c.Width))
	visDelay := defaultVisibilityDelay
	if c.VisibilityDelay != nil {
		visDelay = *c.VisibilityDelay
	}
	opts = append(opts, VisibilityDelay(visDelay), MaxFrameRate(c.MaxFrameRate))
	opts = append(opts, MaxEntities(c.MaxEntities), MaxEvents(c.MaxEvents))
	opts = append(opts, DebugLevel(c.Debug.Level))
	if c.Debug.AddSource {
		opts = append(opts, DebugAddSource())
	}
	if c.Debug.View == DebugPresentationPane {
		var paneOpts []DebugPaneOption
		if c.Debug.PaneHeight > 0 {
			paneOpts = append(paneOpts, PaneHeight(c.Debug.PaneHeight))
		}
		if c.Debug.NewestFirst != nil {
			if *c.Debug.NewestFirst {
				paneOpts = append(paneOpts, NewestFirst())
			} else {
				paneOpts = append(paneOpts, OldestFirst())
			}
		}
		if c.Debug.PreserveAlways {
			paneOpts = append(paneOpts, PreserveDebugTail())
		}
		opts = append(opts, DebugPane(paneOpts...))
	} else {
		opts = append(opts, DebugHistory())
	}
	if c.Strict {
		opts = append(opts, Strict())
	}
	opts = append(opts, withVerbosity(c.Verbosity))
	if c.FailedExitCode != 0 {
		opts = append(opts, withFailedExitCode(c.FailedExitCode))
	}
	if c.DryRun {
		opts = append(opts, DryRun())
	}
	opts = append(opts, Glyphs(c.Glyphs))
	return opts
}

// withVerbosity stores verbosity on the internal config.
func withVerbosity(v Verbosity) Option {
	return optionFunc(func(c *config) { c.verbosity = v })
}

// withFailedExitCode stores a non-default failed conclusion exit code.
func withFailedExitCode(code int) Option {
	return optionFunc(func(c *config) { c.failedExitCode = code })
}

// Title sets the conclusion subject for Config.Options's raw Option path.
func Title(subject string) Option {
	return optionFunc(func(c *config) { c.subject = subject })
}
