package project

import (
	"bytes"
	"io"
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// childWriterMaxPending bounds the unterminated fragment a ChildWriter
// keeps: a child that never ends a line would otherwise grow it without
// limit. Past it the fragment is dropped once shown; every byte still
// reached the evidence, so only the line grouping is lost.
const childWriterMaxPending = 4 * 1024

// ChildWriter narrates a talkative child process: every byte goes to the
// Task's Capture ring, each completed line (LF or CRLF) to line, and a
// fragment still being written, or redrawn in place behind a lone CR (a
// progress bar), to fragment. Lines and fragments arrive trimmed and
// non-empty. It is safe for concurrent use.
type ChildWriter struct {
	evidence *record.Capture
	// line receives a completed line and the ring's line count after it;
	// fragment a line still being written or redrawn in place.
	line     func(text string, retained int)
	fragment func(text string)

	mu  sync.Mutex
	buf []byte
}

// NewChildWriter narrates into evidence, line and fragment.
func NewChildWriter(evidence *record.Capture, line func(text string, retained int), fragment func(text string)) *ChildWriter {
	return &ChildWriter{evidence: evidence, line: line, fragment: fragment}
}

func (w *ChildWriter) Write(p []byte) (int, error) {
	if w.evidence != nil {
		_, _ = w.evidence.Write(p)
	}
	retained := w.evidence.RetainedLines()

	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		text := strings.TrimSpace(string(w.buf[:i]))
		completed, width := lineEnding(w.buf[i:])
		w.buf = w.buf[i+width:]
		switch {
		case text == "":
		case completed:
			w.line(text, retained)
		default:
			w.fragment(text)
		}
	}
	if leftover := strings.TrimSpace(string(w.buf)); leftover != "" {
		w.fragment(leftover)
		if len(w.buf) > childWriterMaxPending {
			w.buf = w.buf[:0]
		}
	}
	return len(p), nil
}

// lineEnding reads the delimiter at the head of rest: LF or CRLF completes
// a line; a lone CR is a frame redrawn in place. width is the delimiter's
// byte count.
func lineEnding(rest []byte) (completed bool, width int) {
	if rest[0] == '\n' {
		return true, 1
	}
	if len(rest) > 1 && rest[1] == '\n' {
		return true, 2
	}
	return false, 1
}

var _ io.Writer = (*ChildWriter)(nil)
