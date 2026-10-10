package engine

import (
	"log/slog"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// SlogHandler returns a slog.Handler that journals every accepted record as
// structured diagnostics (history or pane), without mutating Output configuration.
//
// Level policy is Config.Debug.Level (one conductor):
//
//	out := evo.Init(evo.Config{
//	    Debug: evo.DebugConfig{Level: evo.LevelDebug},
//	})
//	logger := slog.New(out.SlogHandlerForTest())
//
// Application human prose uses Print/Printf/Println or Task outcomes — not slog.
// Infrastructure diagnostics use slog through this handler.
func (o *Output) slogHandler() slog.Handler {
	level := slog.LevelInfo
	if o != nil {
		o.mu.Lock()
		level = logLevelToSlog(o.cfg.debugLevel)
		o.mu.Unlock()
	}
	return record.NewSlogHandler(level, o.emitLogRecord)
}

func logLevelToSlog(l LogLevel) slog.Level {
	switch l {
	case LevelTrace:
		return slog.LevelDebug - 4
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// emitLogRecord journals every accepted slog record as structured diagnostics.
// Info/Warn/Error keep time, level, and attrs on the rendered fields — the raw
// PC always lives on the LogRecord itself (see SlogHandler callers that want
// it), but a bare pc=<uintptr> never reaches human/pane/history rendering
// (release-gate round 6 finding 2). A source=file.go:line field is added
// instead, only when Config.Debug.AddSource opts in.
func (o *Output) emitLogRecord(rec LogRecord) {
	if o == nil {
		return
	}
	fields := make([]Field, 0, len(rec.Attrs)+1)
	for _, a := range rec.Attrs {
		fields = append(fields, Field{Key: a.Key, Value: a.Value.Any()})
	}
	// Force: Handler.Enabled already applied Config.Debug.Level via min.
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cfg.debugAddSource && rec.PC != 0 {
		if source := record.CallerLocation(rec.PC); source != "" {
			fields = append(fields, Field{Key: "source", Value: source})
		}
	}
	o.emitDebugRecordLocked(record.SlogLevelName(rec.Level), rec.Message, fields, rec.Time, true)
}
