package evo

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode/utf8"

	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// Doing, Progress, Writer, Fact and Summary live on Task and Output.

// --- folded from print.go ---

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

// Print formats like fmt.Sprint and enqueues human-facing text (line-buffered).
// Errors are recorded on the Output and returned by Finish/Main — not ignored mid-stream.
func (o *Run) print(args ...any) {
	o.at(visibilityNormal).print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text (line-buffered).
func (o *Run) printf(format string, args ...any) {
	o.at(visibilityNormal).printf(format, args...)
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

// About names what this run is acting on without emitting a row. Call it
// before DeclareDryRun so the header reads "[dry-run] repo  /path" instead
// of the generic marker. Hosts that Init without a path (zq) used to drop
// the name; the library that knows the path should call About.
func (o *Run) about(text string) {
	if o == nil || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cfg.dryRunHeaderText = text
}

// Print implements Printer.
func (p *printer) print(args ...any) {
	p.enqueue(fmt.Sprint(args...))
}

// Printf implements Printer.
func (p *printer) printf(format string, args ...any) {
	p.enqueue(fmt.Sprintf(format, args...))
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
	o.rec.AppendMessage(messageSnapshot{ID: id, Text: line, Visibility: vis})
	// Compatibility: Lines is derived projection of normal+verbose-when-shown.
	if o.projectsVisibilityLocked(vis) {
		o.rec.AppendLine(line)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{
		Type:     "message.emitted",
		EntityID: id,
		Name:     record.VisibilityName(vis),
		// State field reused as visibility tag in JSONL path via Name
	})
	if o.projectsVisibilityLocked(vis) {
		o.emitLineProgressiveLocked()
	} else {
		// Hidden verbose: still count as "emitted" for residual bookkeeping of lines.
		o.linesEmitted = o.rec.LineCount()
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

// --- folded from slog.go ---

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

// --- folded from writer.go ---

// writerIsCharDevice is the TTY-detection facade construction uses.
func writerIsCharDevice(w io.Writer) bool { return terminal.WriterIsCharDevice(w) }

// MarkWriterAsCharDevice treats w as a TTY during construction so tests can
// exercise live-region / color inference without opening a pty. Only this
// writer matches; other tests' writers still use the OS check.
func markWriterAsCharDevice(w io.Writer) func() { return terminal.MarkCharDevice(w) }

// IsCharDevice reports whether w is an *os.File backed by a character device
// (typical interactive TTY). Pipes, files, and non-file writers return false.
//
// Prefer Config{Stdout, Stderr} for ordinary dual-stream construction. This
// helper remains for hosts that choose Plain/NoColor from a concrete writer.
func isCharDevice(w io.Writer) bool { return terminal.IsCharDevice(w) }

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
