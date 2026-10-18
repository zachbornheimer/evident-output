// This file owns Capture: the output a Task's child processes wrote, kept
// sanitized, redacted and bounded so a failure can show its tail.

package record

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// maxCaptureLineLen is the most runes one retained line keeps after
// sanitizing; a longer line ends in an ellipsis.
const maxCaptureLineLen = 4096

// captureEllipsis marks a retained line that was cut.
const captureEllipsis = "…"

// CaptureConfig configures a Capture. A zero bound selects the process
// default; a nil func does nothing.
type CaptureConfig struct {
	MaxLines int
	MaxBytes int
	// Redact removes secrets from each line before it is kept.
	Redact func(string) string
	// OnLine hears each completed line once it is kept.
	OnLine func(line string)
}

// Capture is a Task's retained child-process output: one bounded ring fed by
// the combined stream and by stdout and stderr separately. Every line is
// sanitized, redacted and cut to a bounded width before it is kept. A nil
// Capture keeps nothing.
type Capture struct {
	ring   *process.Ring
	redact func(string) string
}

// NewCapture is an empty Capture.
func NewCapture(cfg CaptureConfig) *Capture {
	c := &Capture{ring: process.NewRing(cfg.MaxLines, cfg.MaxBytes), redact: cfg.Redact}
	if cfg.OnLine != nil {
		c.ring.SetOnAppend(cfg.OnLine)
	}
	return c
}

// captureStream is one process stream of a Capture, as a writer.
type captureStream struct {
	capture *Capture
	stream  process.Stream
}

func (s captureStream) Write(p []byte) (int, error) {
	return s.capture.ring.Write(p, s.stream, s.capture.normalize)
}

// Close keeps the stream's trailing partial line.
func (s captureStream) Close() error {
	return s.capture.ring.Close([]process.Stream{s.stream}, s.capture.normalize)
}

// Stdout is the writer a child's standard output goes to.
func (c *Capture) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return captureStream{capture: c, stream: process.StreamStdout}
}

// Stderr is the writer a child's standard error goes to.
func (c *Capture) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return captureStream{capture: c, stream: process.StreamStderr}
}

// Write keeps p as combined output.
func (c *Capture) Write(p []byte) (int, error) {
	if c == nil {
		return len(p), nil
	}
	return c.ring.Write(p, process.StreamCombined, c.normalize)
}

// Close keeps every stream's trailing partial line.
func (c *Capture) Close() error {
	if c == nil {
		return nil
	}
	return c.ring.Close([]process.Stream{
		process.StreamCombined, process.StreamStdout, process.StreamStderr,
	}, c.normalize)
}

// Text is every kept combined line, after a truncation marker when lines
// were dropped.
func (c *Capture) Text() string {
	lines, truncated := c.texts(process.StreamCombined)
	return process.JoinLines(lines, truncated)
}

// StreamText is every kept line of stream, without a marker.
func (c *Capture) StreamText(stream process.Stream) string {
	lines, _ := c.texts(stream)
	return strings.Join(lines, "\n")
}

// Truncated reports whether the ring dropped a line.
func (c *Capture) Truncated() bool {
	return c != nil && c.ring.Truncated()
}

// Empty reports whether nothing was kept.
func (c *Capture) Empty() bool {
	return c == nil || c.ring.Empty()
}

// RetainedLines is how many lines the ring holds right now. It takes only
// the ring's own lock.
func (c *Capture) RetainedLines() int {
	if c == nil {
		return 0
	}
	return c.ring.Len()
}

// DetailText is the tail a failed Task shows: stderr when the child wrote
// any, else the combined lines, after a truncation marker and a count.
func (c *Capture) DetailText() string {
	if c == nil {
		return ""
	}
	prefer := process.StreamCombined
	if c.ring.PreferStderr() || c.ring.HasStream(process.StreamStderr) {
		prefer = process.StreamStderr
	}
	texts, truncated := c.ring.Texts(prefer, 0, c.normalize)
	if len(texts) == 0 {
		texts, truncated = c.texts(process.StreamCombined)
	}
	if len(texts) == 0 {
		return ""
	}
	var b strings.Builder
	if truncated {
		b.WriteString(process.TruncationMarker)
		b.WriteByte('\n')
	}
	if len(texts) > 1 {
		fmt.Fprintf(&b, "Last %d lines:\n", len(texts))
	}
	b.WriteString(strings.Join(texts, "\n"))
	return b.String()
}

func (c *Capture) texts(stream process.Stream) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	return c.ring.Texts(stream, 0, c.normalize)
}

// normalize is one raw line as it is kept: cut to the chunk bound, valid
// UTF-8, sanitized, redacted and cut to the line width.
func (c *Capture) normalize(line string) string {
	cut := len(line) > process.MaxCaptureChunk
	if cut {
		line = line[:process.MaxCaptureChunk]
	}
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("�")))
	}
	line = SanitizeText(line)
	if c.redact != nil {
		line = c.redact(line)
	}
	line = TruncateUTF8(line, maxCaptureLineLen, captureEllipsis)
	if cut && !strings.HasSuffix(line, captureEllipsis) {
		line += captureEllipsis
	}
	return line
}

var _ io.WriteCloser = (*Capture)(nil)
