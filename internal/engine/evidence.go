package engine

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/record"
)

const (
	defaultCaptureLines = process.DefaultCaptureLines
	defaultCaptureBytes = process.DefaultCaptureBytes
	maxCaptureLineLen   = 4096
	maxCaptureChunk     = process.MaxCaptureChunk
	truncationMarker    = process.TruncationMarker
)

// CaptureStream identifies which process stream a line came from.
type CaptureStream = process.Stream

const (
	CaptureStreamCombined = process.StreamCombined
	CaptureStreamStdout   = process.StreamStdout
	CaptureStreamStderr   = process.StreamStderr
)

// evidence is the retained/redacted process-output sink owned by a Task
// (preferred) or Output. The bounded ring lives in process; this type owns
// sanitization, redaction, mirroring, and Task attachment.
type evidence struct {
	out      *Output
	taskID   string
	taskName string
	ring     *process.Ring
	maxLines int
	maxBytes int

	mirrorDiag  bool
	mirrorDebug bool
	stream      CaptureStream
	parent      *evidence
	onLine      func(text string)
}

// CaptureOption configures Capture.
type CaptureOption interface {
	applyCapture(*evidence)
}

type captureOptionFunc func(*evidence)

func (f captureOptionFunc) applyCapture(c *evidence) { f(c) }

func keepLastLines(n int) CaptureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

func maxCaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

func mirrorToDiagnostics() CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDiag = true })
}

func mirrorToDebug() CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDebug = true })
}

func activityFeed(fn func(text string)) CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.onLine = fn })
}

func (t *TaskHandle) Capture(opts ...CaptureOption) *evidence {
	if t == nil || t.out == nil {
		return newCapture(nil, "", "", opts...)
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskStates[t.id]
	if st == nil {
		return newCapture(t.out, t.id, "", opts...)
	}
	if st.evidence == nil {
		st.evidence = newCapture(t.out, t.id, st.name, opts...)
	}
	return st.evidence
}

func (o *Output) capture(opts ...CaptureOption) *evidence {
	return newCapture(o, "", "", opts...)
}

func newCapture(out *Output, taskID, taskName string, opts ...CaptureOption) *evidence {
	c := &evidence{
		out:         out,
		taskID:      taskID,
		taskName:    taskName,
		maxLines:    defaultCaptureLines,
		maxBytes:    defaultCaptureBytes,
		mirrorDiag:  false,
		mirrorDebug: false,
		stream:      CaptureStreamCombined,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(c)
		}
	}
	c.ring = process.NewRing(c.maxLines, c.maxBytes)
	c.ring.SetOnAppend(func(line string) {
		if c.onLine != nil {
			c.onLine(line)
		}
		if c.out != nil && (c.mirrorDiag || c.mirrorDebug) {
			c.out.mirrorCaptureLine(c.mirrorDiag, c.mirrorDebug, c.taskName, line)
		}
	})
	return c
}

func (c *evidence) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: CaptureStreamStdout, ring: c.ring}
}

func (c *evidence) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: CaptureStreamStderr, ring: c.ring}
}

func (c *evidence) Write(p []byte) (int, error) {
	root := c.root()
	if root == nil || root.ring == nil {
		return len(p), nil
	}
	stream := CaptureStreamCombined
	if c.parent != nil {
		stream = c.stream
	}
	return root.ring.Write(p, stream, root.normalizeCaptureLine)
}

func (c *evidence) Close() error {
	root := c.root()
	if root == nil || root.ring == nil {
		return nil
	}
	if c.parent != nil {
		return root.ring.Close([]process.Stream{c.stream}, root.normalizeCaptureLine)
	}
	return root.ring.Close([]process.Stream{
		CaptureStreamCombined, CaptureStreamStdout, CaptureStreamStderr,
	}, root.normalizeCaptureLine)
}

func (c *evidence) root() *evidence {
	if c == nil {
		return nil
	}
	if c.parent != nil {
		return c.parent
	}
	return c
}

func (c *evidence) Text() string {
	lines, truncated := c.snapshotTexts(CaptureStreamCombined, 0)
	return process.JoinLines(lines, truncated)
}

func (c *evidence) streamText(stream CaptureStream) string {
	lines, _ := c.snapshotTexts(stream, 0)
	return strings.Join(lines, "\n")
}

func (c *evidence) wasTruncated() bool {
	root := c.root()
	if root == nil || root.ring == nil {
		return false
	}
	return root.ring.Truncated()
}

func (c *evidence) Empty() bool {
	root := c.root()
	if root == nil || root.ring == nil {
		return true
	}
	return root.ring.Empty()
}

func (c *evidence) DetailTail() ProblemOption {
	return record.WithEvidenceTail(c.detailText)
}

func (c *evidence) detailText() string {
	root := c.root()
	if root == nil || root.ring == nil {
		return ""
	}
	prefer := CaptureStreamCombined
	if c.parent != nil {
		prefer = c.stream
	} else if root.ring.PreferStderr() || root.ring.HasStream(CaptureStreamStderr) {
		prefer = CaptureStreamStderr
	}
	texts, truncated := root.ring.Texts(prefer, 0, root.normalizeCaptureLine)
	if len(texts) == 0 {
		texts, truncated = root.ring.Texts(CaptureStreamCombined, 0, root.normalizeCaptureLine)
	}
	if len(texts) == 0 {
		return ""
	}
	var b strings.Builder
	if truncated {
		b.WriteString(truncationMarker)
		b.WriteByte('\n')
	}
	if len(texts) > 1 {
		fmt.Fprintf(&b, "Last %d lines:\n", len(texts))
	}
	b.WriteString(strings.Join(texts, "\n"))
	return b.String()
}

func (c *evidence) snapshotTexts(stream CaptureStream, limit int) ([]string, bool) {
	root := c.root()
	if root == nil || root.ring == nil {
		return nil, false
	}
	return root.ring.Texts(stream, limit, root.normalizeCaptureLine)
}

func (c *evidence) normalizeCaptureLine(line string) string {
	cut := len(line) > maxCaptureChunk
	if cut {
		line = line[:maxCaptureChunk]
	}
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = record.SanitizeText(line)
	if c.out != nil {
		line = c.out.redactString(line)
	}
	line = record.TruncateUTF8(line, maxCaptureLineLen, "…")
	if cut && !strings.HasSuffix(line, "…") {
		line += "…"
	}
	return line
}

func (o *Output) mirrorCaptureLine(mirrorDiag, mirrorDebug bool, taskName, line string) {
	if o == nil {
		return
	}
	if mirrorDiag {
		o.writeDiagnosticText(line + "\n")
	}
	if !mirrorDebug {
		return
	}
	o.mu.Lock()
	allowDebug := o.cfg.debugLevel <= LevelDebug
	hasDiag := o.cfg.diagnostic != nil
	interactive := false
	if live := o.liveLocked(); live != nil {
		interactive = live.IsInteractive() && !o.cfg.plain
	}
	o.mu.Unlock()
	if !allowDebug {
		return
	}
	if !hasDiag && !interactive {
		return
	}
	if taskName != "" {
		o.debug(line, Field{Key: "task", Value: taskName})
	} else {
		o.debug(line)
	}
}

var _ io.WriteCloser = (*evidence)(nil)
