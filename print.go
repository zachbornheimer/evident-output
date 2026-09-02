package evo

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Visibility selects whether a message is ordinary or verbose user detail.
// Zero is Normal.
type Visibility uint8

const (
	// Normal messages always project at VerbosityNormal.
	Normal Visibility = iota
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	VisibilityVerbose
)

const (
	maxPendingPrintBytes = 64 * 1024
	pendingTruncMarker   = "…[line truncated]"
)

// MessageSnapshot is one logical user-facing message in the canonical model.
type MessageSnapshot struct {
	ID         string
	Text       string
	Visibility Visibility
}

// Printer is a visibility-scoped view of Output for Print/Printf/Println.
type Printer struct {
	out        *Output
	visibility Visibility
}

// At returns a printer for the given visibility.
func (o *Output) At(visibility Visibility) *Printer {
	return &Printer{out: o, visibility: visibility}
}

// Verbose is sugar for At(VisibilityVerbose).
func (o *Output) Verbose() *Printer {
	return o.At(VisibilityVerbose)
}

// Print formats like fmt.Sprint and enqueues human-facing text (line-buffered).
// Errors are recorded on the Output and returned by Finish/Main — not ignored mid-stream.
func (o *Output) Print(args ...any) {
	o.At(Normal).Print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text (line-buffered).
func (o *Output) Printf(format string, args ...any) {
	o.At(Normal).Printf(format, args...)
}

// Println formats like fmt.Sprintln and enqueues a complete human-facing line.
func (o *Output) Println(args ...any) {
	o.At(Normal).Println(args...)
}

// Print implements Printer.
func (p *Printer) Print(args ...any) {
	p.enqueue(fmt.Sprint(args...))
}

// Printf implements Printer.
func (p *Printer) Printf(format string, args ...any) {
	p.enqueue(fmt.Sprintf(format, args...))
}

// Println implements Printer.
func (p *Printer) Println(args ...any) {
	p.enqueue(fmt.Sprintln(args...))
}

// Writer returns an io.Writer that feeds this printer's line buffer (human stream).
func (o *Output) Writer() io.Writer {
	return o.At(Normal).Writer()
}

// ResultWriter returns the domain-payload stream. Presentation never writes here.
//
// In FormatData mode this is Config.Result if set, otherwise Config.Stdout —
// so machine JSON stays pure while Items/Tasks render on stderr.
// When no result stream is configured, returns io.Discard.
//
//	out := evo.New(evo.Config{Title: "build", Format: evo.FormatData})
//	// after work succeeds:
//	_ = json.NewEncoder(out.ResultWriter()).Encode(payload)
func (o *Output) ResultWriter() io.Writer {
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
func (p *Printer) Writer() io.Writer {
	return &printWriter{p: p}
}

type printWriter struct {
	p *Printer
}

func (w *printWriter) Write(b []byte) (int, error) {
	if w.p == nil {
		return 0, fmt.Errorf("evo: nil printer")
	}
	w.p.enqueue(string(b))
	return len(b), nil
}

func (p *Printer) enqueue(s string) {
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
				line = truncateUTF8(line, maxPendingPrintBytes, pendingTruncMarker)
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
		line = truncateUTF8(line, maxPendingPrintBytes, pendingTruncMarker)
		p.out.emitMessageLocked(line, p.visibility)
		s = s[i+1:]
	}
}

func (o *Output) emitMessageLocked(line string, vis Visibility) {
	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "\uFFFD")
	}
	line = sanitize.Text(line)
	// Drop pure empty? Keep empty lines as messages for fmt parity of blank Println.
	id := o.nextID("message")
	st := messageState{
		id:         id,
		text:       line,
		visibility: vis,
	}
	o.messages = append(o.messages, st)
	// Compatibility: Lines is derived projection of normal+verbose-when-shown.
	if o.projectsVisibilityLocked(vis) {
		o.lines = append(o.lines, line)
	}
	o.bumpLocked()
	o.appendEventLocked(Event{
		Type:     "message.emitted",
		EntityID: id,
		Name:     string(visibilityName(vis)),
		// State field reused as visibility tag in JSONL path via Name
	})
	if o.projectsVisibilityLocked(vis) {
		o.emitLineProgressiveLocked()
	} else {
		// Hidden verbose: still count as "emitted" for residual bookkeeping of lines.
		o.linesEmitted = len(o.lines)
	}
}

func visibilityName(v Visibility) string {
	if v == VisibilityVerbose {
		return "verbose"
	}
	return "normal"
}

func (o *Output) projectsVisibilityLocked(v Visibility) bool {
	if v == VisibilityVerbose {
		return o.cfg.verbosity >= VerbosityVerbose
	}
	return true
}

// flushPendingPrintLocked emits any unterminated print buffer at Finish.
func (o *Output) flushPendingPrintLocked() {
	if o.pendingPrint.Len() > 0 {
		line := o.pendingPrint.String()
		o.pendingPrint.Reset()
		vis := o.pendingVis
		o.emitMessageLocked(line, vis)
	}
}

// messageState is an internal durable message record.
type messageState struct {
	id         string
	text       string
	visibility Visibility
}
