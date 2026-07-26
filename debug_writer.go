package evo

import (
	"bytes"
	"io"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// maxDebugLine is the maximum durable debug line length before truncation.
const maxDebugLine = 4096

// DebugWriter returns a line-oriented writer that emits Debug lines on newline.
// Partial UTF-8 sequences are buffered; control bytes are sanitized.
func (o *Output) DebugWriter() io.WriteCloser {
	return &debugWriter{out: o}
}

type debugWriter struct {
	out *Output
	buf bytes.Buffer
}

func (w *debugWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.buf.Write(p)
			break
		}
		w.buf.Write(p[:i])
		w.flushLine()
		p = p[i+1:]
	}
	// Bound buffer growth
	if w.buf.Len() > maxDebugLine*2 {
		w.flushLine()
	}
	return n, nil
}

func (w *debugWriter) Close() error {
	if w.buf.Len() > 0 {
		w.flushLine()
	}
	return nil
}

func (w *debugWriter) flushLine() {
	line := w.buf.String()
	w.buf.Reset()
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = sanitize.Text(line)
	if len(line) > maxDebugLine {
		line = line[:maxDebugLine] + "…"
	}
	w.out.Debug(line)
}
