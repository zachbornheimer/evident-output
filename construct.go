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
	// Level is the minimum debug journal level (default LevelInfo).
	// Use LevelDebug to surface Debug/Capture debug mirrors.
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

	// Stdout is the ordinary human/result stream (default os.Stdout).
	Stdout io.Writer
	// Stderr owns diagnostics by default (default os.Stderr).
	Stderr io.Writer

	// Verbosity gates Verbose() print messages (default VerbosityNormal).
	Verbosity Verbosity
	// Color policy (default ColorAuto).
	Color ColorMode
	// Format projection mode (default FormatHuman).
	Format Format

	// Debug configures the debug journal.
	Debug DebugConfig

	// Advanced (optional) — zero values inherit safe defaults.
	Clock           TimeSource
	Redactor        Redactor
	Terminal        TerminalDriver
	Strict          bool
	Width           int
	VisibilityDelay time.Duration
	MaxFrameRate    int
	MaxEntities     int
	MaxEvents       int
	// ForcePlain disables live interactive frames even on a TTY.
	ForcePlain bool
	// NonInteractive disables live frames.
	NonInteractive bool
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
		VisibilityDelay: defaultVisibilityDelay,
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
	if c.Debug.Level == 0 && base.Debug.Level != 0 {
		// LevelTrace is 0; treat unset as LevelInfo unless caller set LevelTrace
		// explicitly via NewWithOptions. For Config, 0 means LevelInfo.
		c.Debug.Level = LevelInfo
	}
	if c.Width <= 0 {
		c.Width = base.Width
	}
	if c.VisibilityDelay == 0 {
		c.VisibilityDelay = base.VisibilityDelay
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
		// Human presentation on stderr; domain payload remains on stdout (app-owned).
		opts = append(opts, To(c.Stderr), Diagnostics(c.Stderr), DataProjection())
	case FormatExternal:
		opts = append(opts, To(c.Stdout), Diagnostics(c.Stderr), ExternalProjection())
	default:
		opts = append(opts, To(c.Stdout), Diagnostics(c.Stderr))
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
		opts = append(opts, Terminal(terminal.NewANSI(liveWriter,
			terminal.WithInteractive(true),
			terminal.WithSize(c.Width, 24),
		)))
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
	opts = append(opts, VisibilityDelay(c.VisibilityDelay), MaxFrameRate(c.MaxFrameRate))
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

// NewWithOptions is the advanced Option-based constructor (compatibility path
// for tests and specialized embedding). Prefer New / New(Config) in application code.
func NewWithOptions(options ...Option) *Output {
	return newOutput("", options...)
}

// For creates an Output with a title/subject.
//
// Deprecated: use New(Config{Title: subject}) or New(Config{Title: subject}) with options
// via fields. For remains as a thin compatibility wrapper.
func For(subject string, options ...Option) *Output {
	return newOutput(subject, options...)
}

// NewWithConfig builds an Output from Config.
//
// Deprecated: use New(cfg) which never returns an error for ordinary configs.
func NewWithConfig(cfg Config) (*Output, error) {
	return New(cfg), nil
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
