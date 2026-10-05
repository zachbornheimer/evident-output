package engine

import (
	"io"
	"os"

	"github.com/zachbornheimer/evident-output/terminal"
)

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
	f, isFile := liveWriter.(*os.File)
	if isFile {
		if tw, th, ok := terminal.Size(f); ok {
			// Caller Width>0 is a deterministic override; otherwise use real cols.
			if c.Width <= 0 || c.Width == defaultWidth {
				width = tw
			}
			height = th
		}
	}
	ansiOpts := []terminal.Option{
		terminal.WithInteractive(true),
		terminal.WithSize(width, height),
	}
	if isFile {
		// Re-query geometry on each live redraw (resize-aware path).
		ansiOpts = append(ansiOpts, terminal.WithSizeFile(f))
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
