package evo

import (
	"io"
	"time"

	"github.com/zachbornheimer/evident-output/internal/fs"
)

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

// NoColor disables color.
func withNoColor() option {
	return optionFunc(func(c *config) { c.noColor = true })
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

// DryRun declares this run a dry run: evo.Effect, evo.File, and
// evo.Exec render as
// [planned] rows with imperative verbs instead of [changed] rows with
// past-tense verbs. Set once via Config.DryRun in ordinary application code;
// this Option exists for the advanced NewWithOptions surface and tests.
func dryRun() option {
	return optionFunc(func(c *config) { c.dryRun = true })
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

func maxConcurrency(n int) option {
	return optionFunc(func(c *config) { c.maxConcurrency = n })
}

// withStateDir mirrors Config.StateDir (spec §11.3).
func withStateDir(dir string) option {
	return optionFunc(func(c *config) { c.stateDir = dir })
}

// withFileFS injects the facade evo.File performs filesystem I/O through
// (spec §8.2) — testkit installs a fake so a reconciliation outcome (e.g.
// a chmod failure) is provable without a real disk permission.
func withFileFS(fsys fs.FS) option {
	return optionFunc(func(c *config) { c.fileFS = fsys })
}

// Glyphs selects the glyph capability profile (default GlyphsAuto).
func glyphs(p glyphProfile) option {
	return optionFunc(func(c *config) { c.glyphs = p })
}

// Option configures an Output.
type option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// newOutput builds a run from baseConfig(subject) refined by options:
// the tests' way to set the private config directly.
func newOutput(subject string, options ...option) *Run {
	cfg := baseConfig(subject)
	for _, opt := range options {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	return newRun(cfg)
}
