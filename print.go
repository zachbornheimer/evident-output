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
	// Verbose messages project only when Config.Verbosity is VerbosityVerbose.
	Verbose
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

// Verbose is sugar for At(Verbose).
func (o *Output) Verbose() *Printer {
	return o.At(Verbose)
}

// Print formats like fmt.Sprint and enqueues human-facing text (line-buffered).
func (o *Output) Print(args ...any) (int, error) {
	return o.At(Normal).Print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text (line-buffered).
func (o *Output) Printf(format string, args ...any) (int, error) {
	return o.At(Normal).Printf(format, args...)
}

// Println formats like fmt.Sprintln and enqueues a complete human-facing line.
func (o *Output) Println(args ...any) (int, error) {
	return o.At(Normal).Println(args...)
}

// Print implements Printer.
func (p *Printer) Print(args ...any) (int, error) {
	s := fmt.Sprint(args...)
	return p.enqueue(s)
}

// Printf implements Printer.
func (p *Printer) Printf(format string, args ...any) (int, error) {
	s := fmt.Sprintf(format, args...)
	return p.enqueue(s)
}

// Println implements Printer.
func (p *Printer) Println(args ...any) (int, error) {
	s := fmt.Sprintln(args...)
	return p.enqueue(s)
}

// Writer returns an io.Writer that feeds this printer's line buffer.
func (o *Output) Writer() io.Writer {
	return o.At(Normal).Writer()
}

// Writer returns an io.Writer that feeds this printer's line buffer.
func (p *Printer) Writer() io.Writer {
	return &printWriter{p: p}
}

type printWriter struct {
	p *Printer
}

func (w *printWriter) Write(b []byte) (int, error) {
	n, err := w.p.enqueue(string(b))
	if err != nil {
		return n, err
	}
	return len(b), nil
}

func (p *Printer) enqueue(s string) (int, error) {
	if p == nil || p.out == nil {
		return 0, fmt.Errorf("evo: nil printer")
	}
	n := len(s)
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	if err := p.out.ensureOpen(); err != nil {
		p.out.recordMisuse(err)
		return n, err
	}
	// Normalize CRLF → LF for line splitting.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	buf := &p.out.pendingPrint
	if p.visibility == Verbose {
		buf = &p.out.pendingVerbose
	}
	buf.WriteString(s)
	if buf.Len() > maxPendingPrintBytes {
		// Force-complete with truncation marker.
		line := buf.String()
		buf.Reset()
		if len(line) > maxPendingPrintBytes {
			line = line[:maxPendingPrintBytes] + pendingTruncMarker
		}
		p.out.emitMessageLocked(line, p.visibility)
		return n, nil
	}
	for {
		raw := buf.String()
		i := strings.IndexByte(raw, '\n')
		if i < 0 {
			break
		}
		line := raw[:i]
		buf.Reset()
		buf.WriteString(raw[i+1:])
		p.out.emitMessageLocked(line, p.visibility)
	}
	return n, nil
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
	if v == Verbose {
		return "verbose"
	}
	return "normal"
}

func (o *Output) projectsVisibilityLocked(v Visibility) bool {
	if v == Verbose {
		return o.cfg.verbosity >= VerbosityVerbose
	}
	return true
}

// flushPendingPrintLocked emits any unterminated print buffer at Finish.
func (o *Output) flushPendingPrintLocked() {
	if o.pendingPrint.Len() > 0 {
		line := o.pendingPrint.String()
		o.pendingPrint.Reset()
		o.emitMessageLocked(line, Normal)
	}
	if o.pendingVerbose.Len() > 0 {
		line := o.pendingVerbose.String()
		o.pendingVerbose.Reset()
		o.emitMessageLocked(line, Verbose)
	}
}

// messageState is an internal durable message record.
type messageState struct {
	id         string
	text       string
	visibility Visibility
}
