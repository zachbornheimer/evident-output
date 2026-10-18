package evo

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// visibility selects whether a message is ordinary or verbose user detail; zero is visibilityNormal.
type visibility = record.Visibility

type messageSnapshot = record.MessageSnapshot

// Printer is a visibility-scoped view of Output for Print/Printf/Println.
type printer struct {
	out        *Run
	visibility visibility
}

func (r *Run) at(visibility visibility) *printer {
	return &printer{out: r, visibility: visibility}
}

func (r *Run) println(args ...any) {
	r.at(visibilityNormal).println(args...)
}

// subject prints one durable line immediately, like Config.Subject, for a caller that learns the subject
// only after the run starts but before any other I/O. A no-op on a nil Run or empty text.
func (r *Run) subject(text string) {
	if r == nil || text == "" {
		return
	}
	r.println(text)
}

func (p *printer) println(args ...any) {
	p.enqueue(fmt.Sprintln(args...))
}

func (r *Run) writer() io.Writer {
	return r.at(visibilityNormal).writer()
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

func (r *Run) emitMessageLocked(line string, vis visibility) {
	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "\uFFFD")
	}
	line = txt.Text(line)
	// Empty lines stay as messages so a blank Println matches fmt.
	id := r.graph.NextID("message")
	r.graph.Root().Print(messageSnapshot{ID: id, Text: line, Visibility: vis})
	if r.projectsVisibilityLocked(vis) {
		r.graph.Root().Log(line)
	}
	r.bumpLocked()
	r.appendEventLocked(recEvent{
		Type:     "message.emitted",
		EntityID: id,
		Name:     record.VisibilityName(vis),
	})
	if r.projectsVisibilityLocked(vis) {
		r.show.Stream.Lines()
	} else {
		// Hidden verbose still counts as emitted for lines bookkeeping.
		r.show.Stream.SkipLines()
	}
}

func (r *Run) projectsVisibilityLocked(v visibility) bool {
	if v == visibilityVerbose {
		return r.cfg.verbose
	}
	return true
}

// Text returns a writer for free human text, routed with the presentation so it never tears the live region.
func (r *Run) Text() io.Writer { return r.writer() }

const (
	// visibilityNormal messages always project at VerbosityNormal.
	visibilityNormal = record.VisibilityNormal
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	visibilityVerbose = record.VisibilityVerbose
)
