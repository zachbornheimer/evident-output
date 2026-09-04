package evo

import (
	"fmt"
	"strings"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// DebugPresentation selects how structured debug records project to a TTY (§4.6 / §21.3).
type DebugPresentation int

const (
	// DebugPresentationHistory appends durable scrollback above the live region (default).
	DebugPresentationHistory DebugPresentation = iota
	// DebugPresentationPane keeps a bounded rolling viewport inside the live region.
	DebugPresentationPane
)

const (
	defaultDebugPaneHeight = 5
	debugPaneHeadingNewest = "── debug · newest first ────────────────────────────────────────"
	debugPaneHeadingOldest = "── debug · oldest first ────────────────────────────────────────"
	debugTailHeading       = "── diagnostics ─────────────────────────────────────────────────"
)

// DebugPaneOption configures DebugPane presentation.
type DebugPaneOption interface {
	applyPane(*debugPaneConfig)
}

type debugPaneConfig struct {
	height      int
	newestFirst bool
	// preserveAlways forces a diagnostic tail on every Finish (demo / explicit policy).
	preserveAlways bool
	// preserveOnBad defaults true for pane mode: append tail on failed/blocked/cancelled.
	preserveOnBad bool
}

type paneOptionFunc func(*debugPaneConfig)

func (f paneOptionFunc) applyPane(c *debugPaneConfig) { f(c) }

// PaneHeight sets how many debug records are visible in the pane (not including heading).
func PaneHeight(lines int) DebugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) {
		if lines > 0 {
			c.height = lines
		}
	})
}

// NewestFirst orders the pane with the most recent record first (default).
func NewestFirst() DebugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) { c.newestFirst = true })
}

// OldestFirst orders the pane chronologically (oldest visible first).
func OldestFirst() DebugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) { c.newestFirst = false })
}

// PreserveDebugTail always emits a bounded diagnostic tail under the final report.
// Without this, pane mode still preserves a tail on failed/blocked/cancelled conclusions.
func PreserveDebugTail() DebugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) { c.preserveAlways = true })
}

// DebugHistory selects durable append-above-and-redraw presentation (v0.4 default).
func DebugHistory() Option {
	return optionFunc(func(c *config) {
		c.debugPresentation = DebugPresentationHistory
	})
}

// DebugPane selects a rolling TTY debug viewport at the bottom of the live region.
func DebugPane(opts ...DebugPaneOption) Option {
	return optionFunc(func(c *config) {
		c.debugPresentation = DebugPresentationPane
		cfg := debugPaneConfig{
			height:        defaultDebugPaneHeight,
			newestFirst:   true,
			preserveOnBad: true,
		}
		for _, o := range opts {
			if o != nil {
				o.applyPane(&cfg)
			}
		}
		c.debugPane = cfg
	})
}

// debugRecord is one structured diagnostic journal entry (§21.3).
type debugRecord struct {
	Time    time.Time
	Level   string
	Message string
	Fields  []Field
}

// formatHistoryLine is the compact bracketed grammar for durable scrollback.
// Example: 12:04:18.219 [DEBUG] package index loaded  packages=18
func formatHistoryLine(rec debugRecord, color bool) string {
	levelTok := "[" + rec.Level + "]"
	if color {
		// Spec: color applies primarily to the level token. Warn/Error reuse
		// the same entity glyph palette a Task's own Warning/Failed row uses
		// (stateColor) — one severity vocabulary, not a second one invented
		// for the debug journal (release-gate round 9 finding 3). Debug/Info
		// keep the journal's existing cyan.
		levelTok = txt.Style(levelTok, debugLevelColor(rec.Level), true)
	}
	msg := txt.Text(rec.Message)
	var body string
	if rec.Time.IsZero() {
		body = levelTok + " " + msg
	} else {
		body = rec.Time.Format("15:04:05.000") + " " + levelTok + " " + msg
	}
	if attrs := formatHistoryAttrs(rec.Fields); attrs != "" {
		body += "  " + attrs
	}
	return body
}

// debugLevelColor maps a journal record's level name to the SGR color its
// level token renders with — LevelWarn and LevelError reuse stateColor's
// Warning/Failed colors (txt.SGRYellow/txt.SGRRed); every other level (Debug, Info,
// Trace) keeps the journal's existing cyan.
func debugLevelColor(level string) string {
	switch level {
	case "WARN":
		return txt.SGRYellow
	case "ERROR":
		return txt.SGRRed
	default:
		return txt.SGRCyan
	}
}

func formatHistoryAttrs(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		val := fmt.Sprint(f.Value)
		if f.Sensitive {
			val = "***"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", txt.Text(f.Key), txt.Text(val)))
	}
	return joinArgs(parts)
}

// formatLivePaneLine renders rec for the rolling live pane, narrowing to fit
// columns. One clock, one grammar (release-gate round 6 finding 3): every
// human debug surface — history, live pane, diagnostics tail — renders the
// same bracketed local-time grammar as formatHistoryLine; only the machine
// LogRecord (slog.Record.PC, structured Fields) carries anything else.
// color is always false here — the caller wraps the whole composed line in
// txt.Dim() itself, so the level token is never colored underneath that wrap.
func formatLivePaneLine(rec debugRecord, columns int) string {
	full := formatHistoryLine(rec, false)
	if txt.VisibleCells(full) <= columns {
		return full
	}
	// Too narrow for the full line (attrs included): truncate to the column
	// budget with a trailing "…" rather than silently dropping Fields — the
	// pane must never claim a record had no attributes when it simply ran
	// out of room to show them.
	return txt.Truncate(full, columns)
}

// debugPaneReservedRows is the number of terminal rows the debug pane needs
// (heading + up to height records), used to budget the live task body.
func debugPaneReservedRows(pane debugPaneConfig, recordCount int) int {
	if recordCount <= 0 {
		return 0
	}
	height := pane.height
	if height <= 0 {
		height = defaultDebugPaneHeight
	}
	if recordCount < height {
		height = recordCount
	}
	// blank separator line before heading is written by writeDebugPane's leading \n
	// on body that may already end without newline — count heading + records + 1.
	return height + 2
}

// writeDebugPane appends the rolling pane section to a live-region builder.
func writeDebugPane(b *strings.Builder, records []debugRecord, pane debugPaneConfig, columns int, color bool) {
	if len(records) == 0 {
		return
	}
	height := pane.height
	if height <= 0 {
		height = defaultDebugPaneHeight
	}
	heading := debugPaneHeadingNewest
	if !pane.newestFirst {
		heading = debugPaneHeadingOldest
	}
	if color {
		heading = txt.Dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')

	view := paneView(records, height, pane.newestFirst)
	for _, rec := range view {
		line := formatLivePaneLine(rec, columns)
		if color {
			line = txt.Dim(line, true)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

// paneView returns up to height records in display order.
func paneView(records []debugRecord, height int, newestFirst bool) []debugRecord {
	if height <= 0 || len(records) == 0 {
		return nil
	}
	n := len(records)
	start := 0
	if n > height {
		// Both orderings show the tail window; newestFirst only changes display order (below).
		start = n - height
	}
	slice := records[start:]
	if !newestFirst {
		return slice
	}
	// newest first: reverse the visible window
	out := make([]debugRecord, len(slice))
	for i := range slice {
		out[i] = slice[len(slice)-1-i]
	}
	return out
}

// writeDebugTail appends a labeled diagnostic tail (slog text) after the final report.
func writeDebugTail(b *strings.Builder, records []debugRecord, max int, color bool) {
	if len(records) == 0 {
		return
	}
	if max <= 0 {
		max = defaultDebugPaneHeight
	}
	heading := debugTailHeading
	if color {
		heading = txt.Dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')
	view := paneView(records, max, true) // newest first in failure tail
	for _, rec := range view {
		line := formatHistoryLine(rec, false)
		if color {
			line = txt.Dim(line, true)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func (o *Output) shouldPreserveDebugTailLocked(conc Conclusion) bool {
	// Explicit opt-in always wins (including plain demos with PreserveDebugTail).
	if o.cfg.debugPane.preserveAlways {
		return o.cfg.debugPresentation == DebugPresentationPane && len(o.debugRecords) > 0
	}
	// Default failure tail only when the rolling pane actually owned presentation.
	// History fallback (plain/non-TTY) already streamed durable lines — no second dump.
	if !o.debugPaneActive || o.cfg.debugPresentation != DebugPresentationPane {
		return false
	}
	if !o.cfg.debugPane.preserveOnBad {
		return false
	}
	switch conc.State {
	case StateFailed, StateBlocked, StateCancelled:
		return true
	default:
		return false
	}
}
