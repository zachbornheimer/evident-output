package evo

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	renderplain "github.com/zachbornheimer/evident-output/v2/internal/project/plain"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// visibility selects whether a message is ordinary or verbose user detail; zero is visibilityNormal.
type visibility = record.Visibility

const (
	// visibilityNormal messages always project at VerbosityNormal.
	visibilityNormal = record.VisibilityNormal
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	visibilityVerbose = record.VisibilityVerbose
)

type messageSnapshot = record.MessageSnapshot

// Printer is a visibility-scoped view of Output for Print/Printf/Println.
type printer struct {
	out        *Run
	visibility visibility
}

func (o *Run) at(visibility visibility) *printer {
	return &printer{out: o, visibility: visibility}
}

func (o *Run) println(args ...any) {
	o.at(visibilityNormal).println(args...)
}

// subject prints one durable line immediately, like Config.Subject, for a caller that learns the subject
// only after the run starts but before any other I/O. A no-op on a nil Run or empty text.
func (o *Run) subject(text string) {
	if o == nil || text == "" {
		return
	}
	o.println(text)
}

func (p *printer) println(args ...any) {
	p.enqueue(fmt.Sprintln(args...))
}

func (o *Run) writer() io.Writer {
	return o.at(visibilityNormal).writer()
}

// ResultWriter returns the stdout data payload; presentation never writes to it. Human and Plain runs
// reserve stdout for it and pass the bytes through unchanged. A JSON run embeds what it receives as the
// evo.run document's result, and a JSONL run streams each write as a result.written event, so stdout stays
// machine-readable; both carry bytes that are not valid UTF-8 as base64. Writing after the run finished is
// misuse in every mode and returns an error. A run with no payload stream returns io.Discard.
func (o *Run) ResultWriter() io.Writer {
	if o == nil {
		return io.Discard
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cfg.result != nil || o.cfg.wireFormat == JSON || o.cfg.wireFormat == JSONL {
		return resultWriter{o}
	}
	return io.Discard
}

// resultWriter is the payload stream of a run: it folds a write into the machine document or event stream
// that owns stdout, routes it through the live region when stdout shares the region's terminal, and
// otherwise passes it to stdout as it is.
type resultWriter struct{ o *Run }

func (w resultWriter) Write(b []byte) (int, error) {
	sink, err := w.o.routeResult(b)
	if err != nil {
		return 0, err
	}
	if sink == nil {
		return len(b), nil
	}
	return sink.Write(b)
}

// routeResult delivers b when the run itself owns the destination, and otherwise returns the raw payload
// stream for the caller to write after the run lock is released, so a slow reader never stalls the run.
func (o *Run) routeResult(b []byte) (io.Writer, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return nil, fmt.Errorf("write result: %w", err)
	}
	switch {
	case o.cfg.wireFormat == JSONL:
		o.emitWireEventLocked(wire.EventResultWritten, "", wire.ResultWrittenPayload(b))
	case o.cfg.wireFormat == JSON:
		o.resultText.Write(b)
	case o.cfg.resultSharesTerminal && o.show.Stream.WritePayload(string(b)):
	default:
		return o.cfg.result, nil
	}
	return nil, nil
}

func (p *printer) writer() io.Writer {
	return &printWriter{p: p}
}

type printWriter struct {
	p *printer
}

func (w *printWriter) Write(b []byte) (int, error) {
	if w.p == nil {
		return 0, fmt.Errorf("evo: nil printer")
	}
	w.p.enqueue(string(b))
	return len(b), nil
}

func (p *printer) enqueue(s string) {
	if p == nil || p.out == nil {
		return
	}
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	if err := p.out.ensureOpen(); err != nil {
		p.out.recordMisuse(err)
		return
	}
	p.out.printLines.Add(s, p.visibility)
}

func (o *Run) emitMessageLocked(line string, vis visibility) {
	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "\uFFFD")
	}
	line = txt.Text(line)
	// Empty lines stay as messages so a blank Println matches fmt.
	id := o.graph.NextID("message")
	o.graph.Root().Print(messageSnapshot{ID: id, Text: line, Visibility: vis})
	if o.projectsVisibilityLocked(vis) {
		o.graph.Root().Log(line)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{
		Type:     "message.emitted",
		EntityID: id,
		Name:     record.VisibilityName(vis),
	})
	if o.projectsVisibilityLocked(vis) {
		o.show.Stream.Lines()
	} else {
		// Hidden verbose still counts as emitted for lines bookkeeping.
		o.show.Stream.SkipLines()
	}
}

func (o *Run) projectsVisibilityLocked(v visibility) bool {
	if v == visibilityVerbose {
		return o.cfg.verbose
	}
	return true
}

// slogHandler returns a slog.Handler that journals every accepted record as structured diagnostics. Level
// policy is Config.Debug.Level. Application prose goes through Print/Task outcomes, not slog.
func (o *Run) slogHandler() slog.Handler {
	level := slogLevelOff
	if o != nil {
		o.mu.Lock()
		level = logLevelToSlog(o.cfg.debugLevel)
		o.mu.Unlock()
	}
	return record.NewSlogHandler(level, o.emitLogRecord)
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
// The raw PC stays on the LogRecord; a bare pc=<uintptr> never reaches
// rendering. A source=file.go:line field is added only when Config.Debug.AddSource opts in.
func (o *Run) emitLogRecord(rec logRecord) {
	if o == nil {
		return
	}
	fields := make([]field, 0, len(rec.Attrs)+1)
	for _, a := range rec.Attrs {
		fields = append(fields, field{Key: a.Key, Value: a.Value.Any()})
	}
	// Force: Handler.Enabled already applied Config.Debug.Level via min.
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cfg.debugAddSource && rec.PC != 0 {
		if source := record.CallerLocation(rec.PC); source != "" {
			fields = append(fields, field{Key: "source", Value: source})
		}
	}
	o.emitDebugRecordLocked(record.SlogLevelName(rec.Level), rec.Message, fields, rec.Time, true)
}

// Debug records a structured diagnostic (§4.6 / §21.3). History mode (default) is durable scrollback; pane mode
// shows it in the rolling live pane, durable only if a diagnostic tail is preserved at Finish. With a distinct
// Diagnostics writer, debug lines go there only. Use Capture, not DebugWriter, for Fail Detail.
// debugRecord is one structured diagnostic journal entry (§21.3).
type debugRecord = record.DebugRecord

// emitDebugRecordLocked journals a formed debug record; force bypasses
// DebugLevel after slog already applied its min level.
func (o *Run) emitDebugRecordLocked(levelName, message string, fields []field, at time.Time, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > levelDebug {
		return
	}
	rec := o.newDebugRecordLocked(levelName, message, fields, at)
	o.projectDebugRecordLocked(rec)
}

func (o *Run) newDebugRecordLocked(levelName, message string, fields []field, at time.Time) debugRecord {
	if at.IsZero() {
		at = o.cfg.clock.Now()
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
		} else if o.cfg.redactor != nil {
			rec.Fields[i].Value = o.cfg.redactor.RedactString(fmt.Sprint(rec.Fields[i].Value))
		}
	}
	return rec
}

// projectDebugRecordLocked records rec on the run root, journals it, and hands it to the stream.
func (o *Run) projectDebugRecordLocked(rec debugRecord) {
	o.graph.Root().Debug(rec)
	o.graph.Root().Log(txt.FormatHistoryLine(rec, false))
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "log.emitted"})
	o.show.Stream.Debug(rec)
}

func cloneFields(in []field) []field {
	if len(in) == 0 {
		return nil
	}
	out := make([]field, len(in))
	copy(out, in)
	return out
}

// writerIsCharDevice is the TTY-detection facade construction uses.
func writerIsCharDevice(w io.Writer) bool { return terminal.WriterIsCharDevice(w) }

// sameTerminalDevice reports whether a and b name one physical character device; construction uses it to
// detect Diagnostics sharing the live region's terminal, which writer identity alone misses.
func sameTerminalDevice(a, b io.Writer) bool { return terminal.SameDevice(a, b) }

// Text returns a writer for free human text, routed with the presentation so it never tears the live region.
func (o *Run) Text() io.Writer { return o.writer() }

// SlogHandler returns a slog.Handler that writes into the debug journal at or above Config.Debug.
// See the Run.SlogHandler example.
func (o *Run) SlogHandler() slog.Handler { return o.slogHandler() }

func (r *Run) Summary(text string) *Run {
	r.subject(text)
	return r
}

// Progress sets absolute completed/total progress; use Bytes for byte
// quantities. Prefer it over Advance so retries cannot double-count.
func (t *Task) Progress(completed, total int) *Task {
	return t.setProgress(int64(completed), int64(total), determinate)
}

func (t *Task) setProgress(completed, total int64, kind progressKind) *Task {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied; false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse.
func (t *Task) applyProgressLocked(st *taskState, completed, total int64, kind progressKind) bool {
	verdict, started := st.node.Progress(completed, total, kind, t.out.cfg.clock.Now())
	switch verdict {
	case record.ProgressInvalid:
		t.out.recordMisuse(errInvalidProgress)
		return false
	case record.ProgressRegressed:
		t.out.recordMisuse(errProgressRegression)
		return false
	}
	if started {
		t.out.announceRunningLocked(st)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(recEvent{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.show.Live.Signal(false)
	t.out.show.Stream.Running(st.node.TaskID(), renderplain.TriggerProgress, t.out.cfg.clock.Now())
	return true
}

// Writer returns a line-buffered io.Writer for a talkative child process: each complete line becomes the Task's
// live current item, and every byte is kept so a failure shows the output that explains it. Off a terminal,
// lines never become durable rows. Safe for concurrent use. See the Task.Writer example.
func (t *Task) Writer() io.Writer {
	if t == nil || t.out == nil {
		return io.Discard
	}
	return txt.NewChildWriter(t.capture(), t.appendLiveTail, t.setLiveOnlyPhase)
}
