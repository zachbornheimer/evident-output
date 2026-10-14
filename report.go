package evo

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	txt "github.com/zachbornheimer/evident-output/internal/project"
	renderplain "github.com/zachbornheimer/evident-output/internal/project/plain"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// Doing, Progress, Writer, Fact and Summary live on Task and Output.

// Visibility selects whether a message is ordinary or verbose user detail.
// Zero is VisibilityNormal.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type visibility = record.Visibility

const (
	// VisibilityNormal messages always project at VerbosityNormal (C11:
	// prefixed consistently with VisibilityVerbose — the two enum members
	// previously disagreed on their own naming convention).
	visibilityNormal = record.VisibilityNormal
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	visibilityVerbose = record.VisibilityVerbose
)

const (
	maxPendingPrintBytes = 64 * 1024
	pendingTruncMarker   = "…[line truncated]"
)

// MessageSnapshot is one logical user-facing message in the canonical model.
type messageSnapshot = record.MessageSnapshot

// Printer is a visibility-scoped view of Output for Print/Printf/Println.
type printer struct {
	out        *Run
	visibility visibility
}

// At returns a printer for the given visibility.
func (o *Run) at(visibility visibility) *printer {
	return &printer{out: o, visibility: visibility}
}

// Println formats like fmt.Sprintln and enqueues a complete human-facing line.
func (o *Run) println(args ...any) {
	o.at(visibilityNormal).println(args...)
}

// Subject prints one durable line immediately — the same one-shot semantics
// as Config.Subject, for a caller who doesn't know the subject text until
// after Init (e.g. resolved from a flag), but still before any other I/O
// (I3). A no-op on a nil Output or empty txt.
func (o *Run) subject(text string) {
	if o == nil || text == "" {
		return
	}
	o.println(text)
}

// Println implements Printer.
func (p *printer) println(args ...any) {
	p.enqueue(fmt.Sprintln(args...))
}

// Writer returns an io.Writer that feeds this printer's line buffer (human stream).
func (o *Run) writer() io.Writer {
	return o.at(visibilityNormal).writer()
}

// ResultWriter returns the domain-payload stream. Presentation never writes here.
//
// In FormatData mode this is Config.Result if set, otherwise Config.Stdout —
// so machine JSON stays pure while Tasks render on stderr.
// When no result stream is configured, returns io.Discard.
//
//	out := (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{Name: "build", Output: evo.JSON}, func(ctx context.Context, r *evo.Run) error { return nil })
//	// after work succeeds:
//	_ = json.NewEncoder(out.ResultWriter()).Encode(payload)
func (o *Run) ResultWriter() io.Writer {
	if o == nil {
		return io.Discard
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cfg.result != nil {
		return o.cfg.result
	}
	return io.Discard
}

// Writer returns an io.Writer that feeds this printer's line buffer.
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
	// Normalize CRLF → LF for line splitting.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Single ordered pending stream so Normal/Verbose interleaving preserves call order.
	// Visibility is attached when each complete line is emitted.
	if p.out.pendingVis != p.visibility && p.out.pendingPrint.Len() > 0 {
		// Keep one buffer; visibility switch flushes incomplete fragment as a line.
		line := p.out.pendingPrint.String()
		p.out.pendingPrint.Reset()
		p.out.emitMessageLocked(line, p.out.pendingVis)
	}
	p.out.pendingVis = p.visibility

	// Split complete lines first; only then bound the unfinished fragment.
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			p.out.pendingPrint.WriteString(s)
			if p.out.pendingPrint.Len() > maxPendingPrintBytes {
				line := p.out.pendingPrint.String()
				p.out.pendingPrint.Reset()
				line = txt.TruncateUTF8(line, maxPendingPrintBytes, pendingTruncMarker)
				p.out.emitMessageLocked(line, p.visibility)
			}
			return
		}
		// Complete line = pending fragment + s[:i]
		var line string
		if p.out.pendingPrint.Len() > 0 {
			line = p.out.pendingPrint.String() + s[:i]
			p.out.pendingPrint.Reset()
		} else {
			line = s[:i]
		}
		line = txt.TruncateUTF8(line, maxPendingPrintBytes, pendingTruncMarker)
		p.out.emitMessageLocked(line, p.visibility)
		s = s[i+1:]
	}
}

func (o *Run) emitMessageLocked(line string, vis visibility) {
	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "\uFFFD")
	}
	line = txt.Text(line)
	// Drop pure empty? Keep empty lines as messages for fmt parity of blank Println.
	id := o.graph.NextID("message")
	o.graph.Root().Print(messageSnapshot{ID: id, Text: line, Visibility: vis})
	// Compatibility: Lines is derived projection of normal+verbose-when-shown.
	if o.projectsVisibilityLocked(vis) {
		o.graph.Root().Log(line)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{
		Type:     "message.emitted",
		EntityID: id,
		Name:     record.VisibilityName(vis),
		// State field reused as visibility tag in JSONL path via Name
	})
	if o.projectsVisibilityLocked(vis) {
		o.stream.Lines()
	} else {
		// Hidden verbose: still count as "emitted" for residual bookkeeping of lines.
		o.stream.SkipLines()
	}
}

func (o *Run) projectsVisibilityLocked(v visibility) bool {
	if v == visibilityVerbose {
		return o.cfg.verbosity >= verbosityVerbose
	}
	return true
}

// flushPendingPrintLocked emits any unterminated print buffer at Finish.
func (o *Run) flushPendingPrintLocked() {
	if o.pendingPrint.Len() > 0 {
		line := o.pendingPrint.String()
		o.pendingPrint.Reset()
		vis := o.pendingVis
		o.emitMessageLocked(line, vis)
	}
}

// SlogHandler returns a slog.Handler that journals every accepted record as
// structured diagnostics (history or pane), without mutating Output configuration.
//
// Level policy is Config.Debug.Level (one conductor):
//
//	out := (*evo.Run)(nil); _ = evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error { return nil })
//	logger := slog.New(out.SlogHandlerForTest())
//
// Application human prose uses Print/Printf/Println or Task outcomes — not slog.
// Infrastructure diagnostics use slog through this handler.
func (o *Run) slogHandler() slog.Handler {
	level := slog.LevelInfo
	if o != nil {
		o.mu.Lock()
		level = logLevelToSlog(o.cfg.debugLevel)
		o.mu.Unlock()
	}
	return record.NewSlogHandler(level, o.emitLogRecord)
}

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
		return slog.LevelInfo
	}
}

// logLevelOf is the journal level that keeps l and everything above it:
// the inverse of logLevelToSlog, rounding a level between two named ones
// down so the journal never drops a record Config.Debug asked to keep.
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
// Info/Warn/Error keep time, level, and attrs on the rendered fields — the raw
// PC always lives on the LogRecord itself (see SlogHandler callers that want
// it), but a bare pc=<uintptr> never reaches human/pane/history rendering
// (release-gate round 6 finding 2). A source=file.go:line field is added
// instead, only when Config.Debug.AddSource opts in.
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

// Debug records a structured diagnostic (§4.6 / §21.3).
//
// History mode (default): durable scrollback above the live region (or plain stream).
// Pane mode: record is journaled and shown in the rolling live pane; not durable
// scrollback unless a diagnostic tail is preserved at Finish.
//
// When Diagnostics is configured and is a different writer than the primary stream,
// debug lines go to Diagnostics only (not the human Items/Tasks stream). Use
// Capture for child-process evidence instead of DebugWriter when you need Fail Detail.
// debugRecord is one structured diagnostic journal entry (§21.3).
type debugRecord = record.DebugRecord

func (o *Run) debug(message string, fields ...field) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitDebugLocked(message, fields, false)
}

func (o *Run) emitDebugLocked(message string, fields []field, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > levelDebug {
		return
	}
	rec := o.newDebugRecordLocked("DEBUG", message, fields, time.Time{})
	o.projectDebugRecordLocked(rec)
}

// emitDebugRecordLocked journals a fully-formed debug record (slog bridge).
// force bypasses DebugLevel filtering after slog already applied its min level.
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

// projectDebugRecordLocked records rec through the run root, journals it,
// and hands it to the durable stream to present.
func (o *Run) projectDebugRecordLocked(rec debugRecord) {
	o.graph.Root().Debug(rec)
	o.graph.Root().Log(txt.FormatHistoryLine(rec, false))
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "log.emitted"})
	o.stream.Debug(rec)
}

// writeDiagnosticText emits text on the Diagnostics writer (thread-safe).
func (o *Run) writeDiagnosticText(text string) {
	if o == nil || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writeDiagnosticTextLocked(text)
}

func (o *Run) writeDiagnosticTextLocked(text string) {
	if text != "" {
		humanSinks{o}.Diagnostic(text)
	}
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

// sameTerminalDevice reports whether a and b name one physical character
// device. construct.go uses this at construction time (configToOptions) to
// detect when Diagnostics shares the live region's terminal — never inferred
// later from writer identity alone, which only catches the same-object case.
func sameTerminalDevice(a, b io.Writer) bool { return terminal.SameDevice(a, b) }

func (o *Run) Text() io.Writer { return o.writer() }

func (o *Run) SlogHandler() slog.Handler { return o.slogHandler() }

func (r *Run) Summary(text string) *Run {
	r.subject(text)
	return r
}

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// Prefer absolute Progress over Advance so retries cannot double-count.
func (t *Task) Progress(completed, total int) *Task {
	return t.setProgress(int64(completed), int64(total), determinate)
}

func (t *Task) setProgress(completed, total int64, kind progressKind) *Task {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied — false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse instead, so a paired live update is not applied for a progress change that never happened
// (e.g. Phase) that would otherwise describe a progress change that never
// happened.
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
	t.out.live.Signal(false)
	t.out.stream.Running(st.node.TaskID(), renderplain.TriggerProgress, t.out.cfg.clock.Now())
	return true
}

// Writer returns a line-buffered io.Writer for narrating a talkative child
// process: each complete line (CR or LF terminated, trimmed, non-empty)
// becomes the task's live doing-text (see Doing), and every byte is also
// retained in the task's Capture ring (get-or-create, shared with
// Task.Capture) so DetailTail has proof after Fail. Lines pass through the
// same sanitize layer as Task.Doing, so hostile escape sequences never reach
// the display. Off a TTY, these mirrored lines update the live status only —
// they never force their own durable row the way an explicit
// Task.Doing call does, since the Capture ring (and its failure-path
// DetailTail) is already the child's one durable home (release-gate round 9
// finding 4). Concurrent-safe. Named Writer, not PhaseWriter (P6/rename):
// an io.Writer sink whose lines become the live-status text, following
// logrus's Logger.Writer()/zapio.Writer precedent.
//
//	cmd.Stdout = evo.Task("push").Writer()
func (t *Task) Writer() io.Writer {
	if t == nil || t.out == nil {
		return io.Discard
	}
	return txt.NewChildWriter(t.capture(), t.appendLiveTail, t.setLiveOnlyPhase)
}
