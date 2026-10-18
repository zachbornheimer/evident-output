package evo

import (
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/fs"
)

// option sets one Config or tuning field before newOutput builds the Run through the one constructor.
type option func(*Config, *tuning)

// newOutput builds a run named subject through openRun, as Execute and Main do, with options applied.
func newOutput(subject string, options ...option) *Run {
	cfg := Config{Name: subject}
	var t tuning
	for _, opt := range options {
		if opt != nil {
			opt(&cfg, &t)
		}
	}
	return openRun(cfg, t)
}

// to sets the stream rows and free text reach: stderr.
func to(w io.Writer) option {
	return func(c *Config, _ *tuning) { c.Facades.Stderr = w }
}

// wireFormatOption selects a machine Output, JSON or JSONL.
func wireFormatOption(f Output) option {
	return func(c *Config, _ *tuning) { c.Output = f }
}

// wireStreamOption sets stdout, where a machine Output writes.
func wireStreamOption(w io.Writer) option {
	return func(c *Config, _ *tuning) { c.Facades.Stdout = w }
}

// plain forces the plain projection (no live region).
func plain() option {
	return func(c *Config, _ *tuning) {
		if c.Output != JSON && c.Output != JSONL {
			c.Output = Plain
		}
	}
}

func withNoColor() option {
	return func(_ *Config, t *tuning) { t.color = colorNever }
}

func withClock(ts timeSource) option {
	return func(c *Config, _ *tuning) { c.Facades.Clock = ts }
}

// visibilityDelay sets how long live activity must persist before the first interactive paint.
func visibilityDelay(d time.Duration) option {
	return func(_ *Config, t *tuning) { t.visibilityDelay = delay(d) }
}

func dryRun() option {
	return func(c *Config, _ *tuning) { c.DryRun = true }
}

func stdin(r io.Reader) option {
	return func(c *Config, _ *tuning) { c.Facades.Stdin = r }
}

func withTerminal(driver terminalDriver) option {
	return func(c *Config, _ *tuning) { c.Facades.Terminal = driver }
}

// debugLevel sets the minimum debug journal level.
func debugLevel(level slog.Level) option {
	return func(c *Config, _ *tuning) { c.Debug = level }
}

func maxConcurrency(n int) option {
	return func(c *Config, _ *tuning) { c.MaxConcurrency = n }
}

func withStateDir(dir string) option {
	return func(c *Config, _ *tuning) { c.State.Dir = dir }
}

// withFileFS installs the filesystem evo.File works through, so a fake can prove an outcome a real disk cannot.
func withFileFS(fsys fs.FS) option {
	return func(c *Config, _ *tuning) { c.Facades.FS = fsys }
}
