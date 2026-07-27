package evo

import (
	"fmt"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
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
		// Spec: color applies primarily to the level token.
		levelTok = style(levelTok, sgrCyan, true)
	}
	msg := sanitize.Text(rec.Message)
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
		parts = append(parts, fmt.Sprintf("%s=%s", sanitize.Text(f.Key), sanitize.Text(val)))
	}
	return joinArgs(parts)
}

// formatPaneLine is slog-style text for the rolling pane / diagnostic tail.
// Example: time=2026-07-27T12:04:18.219Z level=DEBUG msg="package index loaded" packages=18
func formatPaneLine(rec debugRecord) string {
	msg := sanitize.Text(rec.Message)
	// Quote msg when it contains spaces (slog text convention).
	if strings.ContainsAny(msg, " \t\"") {
		msg = strconvQuote(msg)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "time=%s level=%s msg=%s",
		rec.Time.UTC().Format(time.RFC3339Nano),
		rec.Level,
		msg,
	)
	for _, f := range rec.Fields {
		val := fmt.Sprint(f.Value)
		if f.Sensitive {
			val = "***"
		}
		key := sanitize.Text(f.Key)
		val = sanitize.Text(val)
		if strings.ContainsAny(val, " \t\"") {
			val = strconvQuote(val)
		}
		fmt.Fprintf(&b, " %s=%s", key, val)
	}
	return b.String()
}

func strconvQuote(s string) string {
	// Minimal quoting without importing strconv for display-only values.
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// writeDebugPane appends the rolling pane section to a live-region builder.
func writeDebugPane(b *strings.Builder, records []debugRecord, pane debugPaneConfig, color bool) {
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
		heading = dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')

	view := paneView(records, height, pane.newestFirst)
	for _, rec := range view {
		line := formatPaneLine(rec)
		if color {
			line = dim(line, true)
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
		if newestFirst {
			start = n - height
		} else {
			// oldest-first viewport: show the oldest height records still in the buffer
			// (full journal is chronological; when truncated, take first height of tail window)
			start = n - height
		}
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
		heading = dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')
	view := paneView(records, max, true) // newest first in failure tail
	for _, rec := range view {
		line := formatPaneLine(rec)
		if color {
			line = dim(line, true)
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
