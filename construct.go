package evo

import (
	"io"
	"os"
	"time"

	"github.com/zachbornheimer/evident-output/terminal"
)

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
	// VerbosityVerbose also projects Verbose-visibility messages.
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
}

// Config is the ordinary application-facing construction surface.
//
// Zero values mean automatic/default behavior. Use DefaultConfig() when you
// need a mutable baseline for advanced fields.
//
//	out := evo.New()
//	out := evo.New(evo.Config{Title: "bpp-csharp"})
//	cfg := evo.DefaultConfig(); cfg.Title = "x"; out := evo.New(cfg)
type Config struct {
	// Title is the subject shown in the conclusion (formerly For's argument).
	Title string

	// Stdout is the ordinary human stream (default os.Stdout).
	// In FormatData mode, Stdout is reserved for domain payload via ResultWriter;
	// human presentation moves to Stderr.
	Stdout io.Writer
	// Stderr owns diagnostics by default (default os.Stderr).
	Stderr io.Writer
	// Result is an optional domain-payload writer. When nil and Format is
	// FormatData, ResultWriter returns Stdout. Presentation never writes here.
	Result io.Writer

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
	// nil means default (150ms). Non-nil is exact, including 0 for immediate.
	// Use evo.Delay(d) to set a value from a duration literal.
	VisibilityDelay *time.Duration
	MaxFrameRate    int
	MaxEntities     int
	MaxEvents       int
	// ForcePlain disables live interactive frames even on a TTY.
	ForcePlain bool
	// NonInteractive disables live frames.
	NonInteractive bool
}

// Delay returns a non-nil *time.Duration for Config fields where zero is meaningful.
//
//	cfg.VisibilityDelay = evo.Delay(0)                      // immediate
//	cfg.VisibilityDelay = evo.Delay(150 * time.Millisecond) // explicit default
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

// New creates an Output from zero or one Config.
//
//	out := evo.New()
//	out := evo.New(evo.Config{Title: "repo"})
//
// More than one Config panics (programmer error). For advanced Option plumbing
// use NewWithOptions.
func New(config ...Config) *Output {
	switch len(config) {
	case 0:
		return newFromConfig(resolveConfig(DefaultConfig()))
	case 1:
		return newFromConfig(resolveConfig(config[0]))
	default:
		panic("evo.New: at most one Config argument")
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

	// Stream routing
	switch c.Format {
	case FormatData:
		// Human presentation on stderr; domain payload on Result (default Stdout).
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
	wantLive := !c.ForcePlain && !c.NonInteractive && c.Format == FormatHuman
	liveWriter := c.Stdout
	if c.Format == FormatData {
		liveWriter = c.Stderr
		wantLive = !c.ForcePlain && !c.NonInteractive && IsCharDevice(c.Stderr)
	} else {
		wantLive = wantLive && IsCharDevice(c.Stdout)
	}
	if c.Terminal != nil {
		opts = append(opts, Terminal(c.Terminal))
	} else if wantLive {
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
				// Cannot establish size safely — fall back to plain progressive.
				wantLive = false
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
		} else {
			opts = append(opts, Plain())
		}
	} else {
		opts = append(opts, Plain())
	}
	if c.NonInteractive {
		opts = append(opts, NonInteractive())
	}
	if c.ForcePlain {
		opts = append(opts, Plain())
	}

	opts = append(opts, Clock(c.Clock), Redact(c.Redactor), Width(c.Width))
	visDelay := defaultVisibilityDelay
	if c.VisibilityDelay != nil {
		visDelay = *c.VisibilityDelay
	}
	opts = append(opts, VisibilityDelay(visDelay), MaxFrameRate(c.MaxFrameRate))
	opts = append(opts, MaxEntities(c.MaxEntities), MaxEvents(c.MaxEvents))
	opts = append(opts, DebugLevel(c.Debug.Level))
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
	return opts
}

// withVerbosity stores verbosity on the internal config.
func withVerbosity(v Verbosity) Option {
	return optionFunc(func(c *config) { c.verbosity = v })
}

// NewWithOptions is the advanced Option-based constructor for tests and
// specialized embedding (custom Terminal, Clock, etc.). Prefer New(Config) in
// application code.
//
// Set the conclusion title with Title(...):
//
//	out := evo.NewWithOptions(evo.Title("install"), evo.To(&buf), evo.Plain())
func NewWithOptions(options ...Option) *Output {
	return newOutput("", options...)
}

// Title sets the conclusion subject for NewWithOptions.
func Title(subject string) Option {
	return optionFunc(func(c *config) { c.subject = subject })
}

// ParseColorMode maps always|never|auto (and common synonyms) to ColorMode.
func ParseColorMode(s string) (ColorMode, error) {
	switch s {
	case "", "auto":
		return ColorAuto, nil
	case "always", "on", "yes", "true", "1":
		return ColorAlways, nil
	case "never", "off", "no", "false", "0":
		return ColorNever, nil
	default:
		return ColorAuto, errInvalidColorMode(s)
	}
}

func errInvalidColorMode(s string) error {
	return &UsageError{Op: "ParseColorMode", Msg: "unknown color mode " + s}
}

// UsageError is a programmer/user configuration error.
type UsageError struct {
	Op  string
	Msg string
}

func (e *UsageError) Error() string {
	if e == nil {
		return "usage error"
	}
	if e.Op == "" {
		return e.Msg
	}
	return e.Op + ": " + e.Msg
}
