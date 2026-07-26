package evo

import (
	"context"
	"log/slog"
)

// SlogHandler returns a slog.Handler that routes records through Output.Debug/Info lines.
func (o *Output) SlogHandler(min slog.Leveler) slog.Handler {
	level := slog.LevelInfo
	if min != nil {
		level = min.Level()
	}
	return &slogBridge{out: o, min: level}
}

type slogBridge struct {
	out *Output
	min slog.Level
	// group prefix for WithGroup
	group string
	attrs []slog.Attr
}

func (h *slogBridge) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.min
}

func (h *slogBridge) Handle(_ context.Context, r slog.Record) error {
	fields := make([]Field, 0, r.NumAttrs()+len(h.attrs))
	for _, a := range h.attrs {
		fields = append(fields, Field{Key: a.Key, Value: a.Value.Any()})
	}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		fields = append(fields, Field{Key: key, Value: a.Value.Any()})
		return true
	})
	msg := r.Message
	switch {
	case r.Level >= slog.LevelError:
		h.out.ErrorMessage(msg, fields...)
	case r.Level >= slog.LevelWarn:
		h.out.WarnMessage(msg, fields...)
	case r.Level >= slog.LevelInfo:
		h.out.Info(msg, fields...)
	default:
		// Ensure debug is not filtered by Output.debugLevel when handler accepted it.
		h.out.mu.Lock()
		prev := h.out.cfg.debugLevel
		if prev > LevelDebug {
			h.out.cfg.debugLevel = LevelDebug
		}
		h.out.mu.Unlock()
		h.out.Debug(msg, fields...)
		h.out.mu.Lock()
		h.out.cfg.debugLevel = prev
		h.out.mu.Unlock()
	}
	return nil
}

func (h *slogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := *h
	cp.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
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
