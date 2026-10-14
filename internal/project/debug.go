package project

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// DefaultDebugPaneHeight is how many records a debug pane shows when its
// height is not set.
const DefaultDebugPaneHeight = 5

const (
	debugPaneHeadingNewest = "── debug · newest first ────────────────────────────────────────"
	debugPaneHeadingOldest = "── debug · oldest first ────────────────────────────────────────"
	debugTailHeading       = "── diagnostics ─────────────────────────────────────────────────"
)

// DebugPane is how the rolling live debug pane (§21.3.2) shows records.
type DebugPane struct {
	Height      int
	NewestFirst bool
	// PreserveAlways forces a diagnostic tail on every Finish (demo or
	// explicit policy).
	PreserveAlways bool
	// PreserveOnBad appends the tail when the run failed, refused or was
	// cancelled; pane mode defaults it on.
	PreserveOnBad bool
}

// Rows is how many records the pane shows: its height, or the default when
// none was set.
func (pane DebugPane) Rows() int {
	if pane.Height <= 0 {
		return DefaultDebugPaneHeight
	}
	return pane.Height
}

// FormatHistoryLine is the compact bracketed grammar for durable scrollback.
// Example: 12:04:18.219 [DEBUG] package index loaded  packages=18
func FormatHistoryLine(rec record.DebugRecord, color bool) string {
	levelTok := "[" + rec.Level + "]"
	if color {
		// Spec: color applies primarily to the level token. Warn/Error reuse
		// the same entity glyph palette a Task's own Warning/Failed row uses
		// (stateColor) — one severity vocabulary, not a second one invented
		// for the debug journal (release-gate round 9 finding 3). Debug/Info
		// keep the journal's existing cyan.
		levelTok = ApplySGR(levelTok, debugLevelColor(rec.Level), true)
	}
	msg := Text(rec.Message)
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
// Warning/Failed colors (SGRYellow/SGRRed); every other level (Debug, Info,
// Trace) keeps the journal's existing cyan.
func debugLevelColor(level string) string {
	switch level {
	case "WARN":
		return SGRYellow
	case "ERROR":
		return SGRRed
	default:
		return SGRCyan
	}
}

func formatHistoryAttrs(fields []record.Field) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		val := fmt.Sprint(f.Value)
		if f.Sensitive {
			val = record.RedactedValue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", Text(f.Key), Text(val)))
	}
	return strings.Join(parts, " ")
}

// formatLivePaneLine renders rec for the rolling live pane, narrowing to fit
// columns. One clock, one grammar (release-gate round 6 finding 3): every
// human debug surface — history, live pane, diagnostics tail — renders the
// same bracketed local-time grammar as FormatHistoryLine; only the machine
// LogRecord (slog.Record.PC, structured Fields) carries anything else.
// color is always false here — the caller wraps the whole composed line in
// Dim() itself, so the level token is never colored underneath that wrap.
func formatLivePaneLine(rec record.DebugRecord, columns int) string {
	full := FormatHistoryLine(rec, false)
	if VisibleCells(full) <= columns {
		return full
	}
	// Too narrow for the full line (attrs included): truncate to the column
	// budget with a trailing "…" rather than silently dropping Fields — the
	// pane must never claim a record had no attributes when it simply ran
	// out of room to show them.
	return Truncate(full, columns)
}

// ReservedRows is the number of terminal rows the pane needs for
// recordCount records (heading + up to its height of records), used to
// budget the live task body.
func (pane DebugPane) ReservedRows(recordCount int) int {
	if recordCount <= 0 {
		return 0
	}
	height := min(recordCount, pane.Rows())
	// blank separator line before heading is written by WriteDebugPane's leading \n
	// on body that may already end without newline — count heading + records + 1.
	return height + 2
}

// WriteDebugPane appends the rolling pane section to a live-region builder.
func WriteDebugPane(b *strings.Builder, records []record.DebugRecord, pane DebugPane, columns int, color bool) {
	if len(records) == 0 {
		return
	}
	height := pane.Rows()
	heading := debugPaneHeadingNewest
	if !pane.NewestFirst {
		heading = debugPaneHeadingOldest
	}
	if color {
		heading = Dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')

	view := paneView(records, height, pane.NewestFirst)
	for _, rec := range view {
		line := formatLivePaneLine(rec, columns)
		if color {
			line = Dim(line, true)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

// paneView returns up to height records in display order.
func paneView(records []record.DebugRecord, height int, newestFirst bool) []record.DebugRecord {
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
	out := make([]record.DebugRecord, len(slice))
	for i := range slice {
		out[i] = slice[len(slice)-1-i]
	}
	return out
}

// WriteDebugTail appends a labeled diagnostic tail (slog text) after the final report.
func WriteDebugTail(b *strings.Builder, records []record.DebugRecord, max int, color bool) {
	if len(records) == 0 {
		return
	}
	if max <= 0 {
		max = DefaultDebugPaneHeight
	}
	heading := debugTailHeading
	if color {
		heading = Dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')
	view := paneView(records, max, true) // newest first in failure tail
	for _, rec := range view {
		line := FormatHistoryLine(rec, false)
		if color {
			line = Dim(line, true)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}
