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
	subject            string
	primary            io.Writer
	diagnostic         io.Writer
	plain              bool
	nonInteractive     bool
	noColor            bool
	width              int
	clock              TimeSource
	visibilityDelay    time.Duration
	maxFrameRate       int
	strict             bool
	terminal           TerminalDriver
	debugLevel         LogLevel
	debugPresentation  DebugPresentation
	debugPane          debugPaneConfig
	redactor           Redactor
	projection         ProjectionPolicy
	maxEntities        int
	maxEvents          int
	extraWriters       []io.Writer
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// To sets the primary human writer.
func To(w io.Writer) Option {
	return optionFunc(func(c *config) { c.primary = w })
}

// Diagnostics sets the diagnostic writer.
func Diagnostics(w io.Writer) Option {
	return optionFunc(func(c *config) { c.diagnostic = w })
}

// Plain forces final-report projection (no live spinner region).
// Semantic color is still emitted unless NoColor is set.
func Plain() Option {
	return optionFunc(func(c *config) { c.plain = true })
}

// NonInteractive disables live interactive frames.
func NonInteractive() Option {
	return optionFunc(func(c *config) { c.nonInteractive = true })
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

// VisibilityDelay sets the spinner visibility threshold.
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

// Terminal injects a terminal driver (interactive projection; v0.2).
func Terminal(driver TerminalDriver) Option {
	return optionFunc(func(c *config) { c.terminal = driver })
}

// LogLevel is a diagnostic severity.
type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

// Debug is the debug log level (Appendix H).
const Debug = LevelDebug

// DebugLevel sets the minimum debug emission level.
func DebugLevel(level LogLevel) Option {
	return optionFunc(func(c *config) { c.debugLevel = level })
}

// TerminalDriver is the exclusive owner of terminal control sequences.
// Interactive implementation arrives in v0.2; the interface is defined early
// so options and tests compile.
type TerminalDriver interface {
	ID() string
}

const (
	defaultVisibilityDelay = 150 * time.Millisecond
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
