package project

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

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
