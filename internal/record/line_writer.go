package record

import (
	"bytes"
	"io"
	"unicode/utf8"
)

// maxLineBytes is the maximum line length a LineWriter emits before truncation.
const maxLineBytes = 4096

// NewLineWriter returns a line-oriented writer that hands emit each line on
// newline. Partial UTF-8 sequences are buffered; control bytes are
// sanitized; a line past maxLineBytes is truncated.
func NewLineWriter(emit func(line string)) io.WriteCloser {
	return &lineWriter{emit: emit}
}

type lineWriter struct {
	emit func(line string)
	buf  bytes.Buffer
}

func (w *lineWriter) Write(p []byte) (int, error) {
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
	if w.buf.Len() > maxLineBytes*2 {
		w.flushLine()
	}
	return n, nil
}

func (w *lineWriter) Close() error {
	if w.buf.Len() > 0 {
		w.flushLine()
	}
	return nil
}

func (w *lineWriter) flushLine() {
	line := w.buf.String()
	w.buf.Reset()
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("�")))
	}
	line = SanitizeText(line)
	line = TruncateUTF8(line, maxLineBytes, "…")
	w.emit(line)
}
