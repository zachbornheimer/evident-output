package record

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
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

// NewSlogHandler returns a slog.Handler that hands sink every record at or
// above min, group-qualified, as a LogRecord.
func NewSlogHandler(min slog.Level, sink func(LogRecord)) slog.Handler {
	return &slogBridge{sink: sink, min: min}
}

type slogBridge struct {
	sink  func(LogRecord)
	min   slog.Level
	group string
	attrs []slog.Attr
}

func (h *slogBridge) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.min
}

func (h *slogBridge) Handle(_ context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0, r.NumAttrs()+len(h.attrs))
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
	h.sink(LogRecord{
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

// CallerLocation resolves a slog record's program counter to "file.go:line",
// matching slog.HandlerOptions.AddSource's own frame lookup.
func CallerLocation(pc uintptr) string {
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.File == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", frame.File, frame.Line)
}

// SlogLevelName is the history-line level token for level.
func SlogLevelName(level slog.Level) string {
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
		return level.String()
	}
}
