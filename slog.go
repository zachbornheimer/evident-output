package evo

import (
	"context"
	"log/slog"
	"time"
)

// LogRecord is a complete internal log entry preserved from slog (or peer bridges).
// History and pane projectors read Time/Level/Message/Attrs without lossy remapping
// beyond the usual Field redaction path.
type LogRecord struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
	PC      uintptr
}

// SlogHandler returns a slog.Handler that journals every accepted record as
// structured diagnostics (history or pane), without mutating Output configuration.
// Time, level name, attributes, and PC are preserved for Info/Warn/Error as well
// as Debug — not demoted to unstructured Line() messages.
//
// Prefer Info / WarnMessage / ErrorMessage for plain human-facing application
// lines that are not log infrastructure.
func (o *Output) SlogHandler(min slog.Leveler) slog.Handler {
	level := slog.LevelInfo
	if min != nil {
		level = min.Level()
	}
	return &slogBridge{out: o, min: level}
}

type slogBridge struct {
	out   *Output
	min   slog.Level
	group string
	attrs []slog.Attr
}

func (h *slogBridge) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.min
}

func (h *slogBridge) Handle(_ context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0, r.NumAttrs()+len(h.attrs))
	// WithAttrs already carry any group prefix applied at WithAttrs time.
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		a.Key = key
		attrs = append(attrs, a)
		return true
	})
	h.out.emitLogRecord(LogRecord{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
		Attrs:   attrs,
		PC:      r.PC,
	})
	return nil
}

func (h *slogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := *h
	prefixed := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if h.group != "" {
			a.Key = h.group + "." + a.Key
		}
		prefixed = append(prefixed, a)
	}
	cp.attrs = append(append([]slog.Attr{}, h.attrs...), prefixed...)
	return &cp
}

func (h *slogBridge) WithGroup(name string) slog.Handler {
	cp := *h
	if h.group == "" {
		cp.group = name
	} else {
		cp.group = h.group + "." + name
	}
	return &cp
}

// emitLogRecord journals every accepted slog record as structured diagnostics.
// Info/Warn/Error are not demoted to unstructured Line() messages — that would
// drop time, level, attrs, and PC. Application code wanting plain human lines
// should call Info / WarnMessage / ErrorMessage directly.
func (o *Output) emitLogRecord(rec LogRecord) {
	if o == nil {
		return
	}
	fields := make([]Field, 0, len(rec.Attrs)+1)
	for _, a := range rec.Attrs {
		fields = append(fields, Field{Key: a.Key, Value: a.Value.Any()})
	}
	// Preserve source PC when present (slog AddSource).
	if rec.PC != 0 {
		fields = append(fields, Field{Key: "pc", Value: rec.PC})
	}
	// Force: slog already applied its min level; do not re-filter by debugLevel.
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitDebugRecordLocked(slogLevelName(rec.Level), rec.Message, fields, rec.Time, true)
}

func slogLevelName(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	case level >= slog.LevelDebug:
		return "DEBUG"
	default:
		// Custom / Trace-ish levels below Debug keep slog's string form.
		return level.String()
	}
}
