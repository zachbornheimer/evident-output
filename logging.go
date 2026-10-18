package evo

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	txt "github.com/zachbornheimer/evident-output/v2/internal/project"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// slogHandler returns a slog.Handler that journals every accepted record as structured diagnostics. Level
// policy is Config.Debug.Level. Application prose goes through Print/Task outcomes, not slog.
func (r *Run) slogHandler() slog.Handler {
	level := slogLevelOff
	if r != nil {
		r.mu.Lock()
		level = logLevelToSlog(r.cfg.debugLevel)
		r.mu.Unlock()
	}
	return record.NewSlogHandler(level, r.emitLogRecord)
}

// slogLevelOff is above every level a record can carry, so a handler at it enables none.
const slogLevelOff = slog.Level(math.MaxInt)

func logLevelToSlog(l logLevel) slog.Level {
	switch l {
	case levelTrace:
		return slog.LevelDebug - 4
	case levelDebug:
		return slog.LevelDebug
	case levelInfo:
		return slog.LevelInfo
	case levelWarn:
		return slog.LevelWarn
	case levelError:
		return slog.LevelError
	default:
		return slogLevelOff
	}
}

// logLevelOf is the journal level keeping l and above: the inverse of logLevelToSlog, rounding down so the
// journal never drops a record Config.Debug asked to keep.
func logLevelOf(l slog.Level) logLevel {
	switch {
	case l < slog.LevelDebug:
		return levelTrace
	case l < slog.LevelInfo:
		return levelDebug
	case l < slog.LevelWarn:
		return levelInfo
	case l < slog.LevelError:
		return levelWarn
	default:
		return levelError
	}
}

// emitLogRecord journals every accepted slog record as structured diagnostics.
// The raw PC stays on the LogRecord; a bare pc=<uintptr> never reaches rendering.
func (r *Run) emitLogRecord(rec logRecord) {
	if r == nil {
		return
	}
	fields := make([]field, 0, len(rec.Attrs)+1)
	for _, a := range rec.Attrs {
		fields = append(fields, field{Key: a.Key, Value: a.Value.Any()})
	}
	// Force: Handler.Enabled already applied Config.Debug.Level via min.
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emitDebugRecordLocked(record.SlogLevelName(rec.Level), rec.Message, fields, rec.Time, true)
}

// debugRecord is one structured diagnostic journal entry.
type debugRecord = record.DebugRecord

// emitDebugRecordLocked journals a formed debug record; force bypasses
// DebugLevel after slog already applied its min level.
func (r *Run) emitDebugRecordLocked(levelName, message string, fields []field, at time.Time, force bool) {
	if err := r.ensureOpen(); err != nil {
		r.recordMisuse(err)
		return
	}
	if !force && r.cfg.debugLevel > levelDebug {
		return
	}
	rec := r.newDebugRecordLocked(levelName, message, fields, at)
	r.projectDebugRecordLocked(rec)
}

func (r *Run) newDebugRecordLocked(levelName, message string, fields []field, at time.Time) debugRecord {
	if at.IsZero() {
		at = r.cfg.clock.Now()
	}
	rec := debugRecord{
		Time:    at,
		Level:   levelName,
		Message: txt.Text(message),
		Fields:  cloneFields(fields),
	}
	for i := range rec.Fields {
		if rec.Fields[i].Sensitive {
			rec.Fields[i].Value = record.RedactedValue
		} else if r.cfg.redactor != nil {
			rec.Fields[i].Value = r.cfg.redactor.RedactString(fmt.Sprint(rec.Fields[i].Value))
		}
	}
	return rec
}

// projectDebugRecordLocked records rec on the run root, journals it, and hands it to the stream.
func (r *Run) projectDebugRecordLocked(rec debugRecord) {
	r.graph.Root().Debug(rec)
	r.graph.Root().Log(txt.FormatHistoryLine(rec, false))
	r.bumpLocked()
	r.appendEventLocked(recEvent{Type: "log.emitted"})
	r.show.Stream.Debug(rec)
}

func cloneFields(in []field) []field {
	if len(in) == 0 {
		return nil
	}
	out := make([]field, len(in))
	copy(out, in)
	return out
}

// SlogHandler returns a slog.Handler that writes into the debug journal at or above Config.Debug.
// See the Run.SlogHandler example.
func (r *Run) SlogHandler() slog.Handler { return r.slogHandler() }
