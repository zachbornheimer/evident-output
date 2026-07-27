package evo

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Default ring bounds for Capture (child process evidence, not a live UI).
const (
	defaultCaptureLines = 200
	defaultCaptureBytes = 256 << 10 // 256 KiB
	maxCaptureLineLen   = 4096
	truncationMarker    = "[earlier output truncated]"
)

// CaptureStream identifies which process stream a line came from.
type CaptureStream uint8

const (
	// CaptureStreamCombined is Write() on the Capture itself (merged by the runner).
	CaptureStreamCombined CaptureStream = iota
	// CaptureStreamStdout is output.Stdout().
	CaptureStreamStdout
	// CaptureStreamStderr is output.Stderr().
	CaptureStreamStderr
)

type capturedLine struct {
	Sequence uint64
	Stream   CaptureStream
	Text     string
}

// Capture is a process-output sink owned by a Task (preferred) or Output.
//
//	upgrade := out.Task("brew packages")
//	output := upgrade.Capture() // silent retention by default
//	if err := run.Run(ctx, "brew", args, output); err != nil {
//	    upgrade.Fail("brew upgrade failed", evo.Cause(err), output.DetailTail())
//	    return nil
//	}
//	upgrade.Done()
//
// Semantics:
//   - Always retains a bounded ring of sanitized lines (evidence exists even when
//     debug presentation is disabled).
//   - Default is silent: no Diagnostics/Debug mirror on success.
//   - Opt in with MirrorToDiagnostics / MirrorToDebug.
//   - Stdout/Stderr have independent pending buffers (no partial-line merge).
//   - DetailTail is user-visible failure evidence; Cause is structured diagnostic.
type Capture struct {
	out      *Output
	taskID   string
	taskName string

	mu sync.Mutex

	// Independent pending line assembly per stream.
	pendingCombined bytes.Buffer
	pendingStdout   bytes.Buffer
	pendingStderr   bytes.Buffer

	// Sequenced combined ring (newest at end).
	lines     []capturedLine
	seq       uint64
	maxLines  int
	maxBytes  int
	nbytes    int
	truncated bool

	// Mirror policy (default: all false — silent retention).
	mirrorDiag  bool
	mirrorDebug bool

	// stream is set only on side writers returned by Stdout/Stderr.
	stream CaptureStream
	parent *Capture
}

// CaptureOption configures Capture.
type CaptureOption interface {
	applyCapture(*Capture)
}

type captureOptionFunc func(*Capture)

func (f captureOptionFunc) applyCapture(c *Capture) { f(c) }

// KeepLastLines sets how many trailing lines are retained (default 200).
func KeepLastLines(n int) CaptureOption { return CaptureLines(n) }

// CaptureLines sets how many trailing lines are retained (default 200).
func CaptureLines(n int) CaptureOption {
	return captureOptionFunc(func(c *Capture) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

// MaxCaptureBytes sets an approximate byte budget for retained lines (default 256KiB).
func MaxCaptureBytes(n int) CaptureOption { return CaptureBytes(n) }

// CaptureBytes sets an approximate byte budget for retained lines (default 256KiB).
func CaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *Capture) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

// MirrorToDiagnostics copies each completed line to the Diagnostics writer.
// Default is off — Capture retains evidence without displaying on success.
func MirrorToDiagnostics() CaptureOption {
	return captureOptionFunc(func(c *Capture) { c.mirrorDiag = true })
}

// MirrorToDebug journals each completed line via Debug when DebugLevel allows.
// Default is off.
func MirrorToDebug() CaptureOption {
	return captureOptionFunc(func(c *Capture) { c.mirrorDebug = true })
}

// CaptureQuiet is retained for compatibility; default is already silent.
// Prefer omitting mirror options instead.
func CaptureQuiet() CaptureOption {
	return captureOptionFunc(func(c *Capture) {
		c.mirrorDiag = false
		c.mirrorDebug = false
	})
}

// Capture returns a process-output sink bound to this Task.
func (t *Task) Capture(opts ...CaptureOption) *Capture {
	if t == nil || t.out == nil {
		return newCapture(nil, "", "", opts...)
	}
	name := ""
	t.out.mu.Lock()
	if st := t.out.taskByRef[t.id]; st != nil {
		name = st.name
	}
	t.out.mu.Unlock()
	return newCapture(t.out, t.id, name, opts...)
}

// Capture returns an unscoped process sink (no owning task).
// Prefer Task.Capture so failure evidence is associated with the operation.
func (o *Output) Capture(opts ...CaptureOption) *Capture {
	return newCapture(o, "", "", opts...)
}

func newCapture(out *Output, taskID, taskName string, opts ...CaptureOption) *Capture {
	c := &Capture{
		out:         out,
		taskID:      taskID,
		taskName:    taskName,
		maxLines:    defaultCaptureLines,
		maxBytes:    defaultCaptureBytes,
		mirrorDiag:  false, // silent by default — release invariant
		mirrorDebug: false,
		stream:      CaptureStreamCombined,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(c)
		}
	}
	return c
}

// Stdout returns a writer that records lines as stdout with its own pending buffer.
func (c *Capture) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &Capture{out: c.out, parent: c, stream: CaptureStreamStdout}
}

// Stderr returns a writer that records lines as stderr with its own pending buffer.
func (c *Capture) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &Capture{out: c.out, parent: c, stream: CaptureStreamStderr}
}

// Write implements io.Writer. Safe for concurrent use with Tail/DetailTail.
func (c *Capture) Write(p []byte) (int, error) {
	root := c.root()
	if root == nil || root.out == nil {
		return len(p), nil
	}
	n := len(p)
	stream := CaptureStreamCombined
	if c.parent != nil {
		stream = c.stream
	}

	root.mu.Lock()
	defer root.mu.Unlock()
	buf := root.pendingFor(stream)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			buf.Write(p)
			break
		}
		buf.Write(p[:i])
		root.flushPendingLocked(stream)
		p = p[i+1:]
	}
	if buf.Len() > maxCaptureLineLen*2 {
		root.flushPendingLocked(stream)
	}
	return n, nil
}

// Close flushes trailing partial lines on this writer’s stream.
func (c *Capture) Close() error {
	root := c.root()
	if root == nil {
		return nil
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	stream := CaptureStreamCombined
	if c.parent != nil {
		stream = c.stream
	}
	if root.pendingFor(stream).Len() > 0 {
		root.flushPendingLocked(stream)
	}
	return nil
}

func (c *Capture) root() *Capture {
	if c == nil {
		return nil
	}
	if c.parent != nil {
		return c.parent
	}
	return c
}

func (c *Capture) pendingFor(stream CaptureStream) *bytes.Buffer {
	switch stream {
	case CaptureStreamStdout:
		return &c.pendingStdout
	case CaptureStreamStderr:
		return &c.pendingStderr
	default:
		return &c.pendingCombined
	}
}

// Text returns all retained combined lines joined by newlines.
func (c *Capture) Text() string {
	lines, truncated := c.snapshotTexts(CaptureStreamCombined, 0)
	return joinCaptureLines(lines, truncated)
}

// Lines returns retained combined line texts (oldest first).
func (c *Capture) Lines() []string {
	lines, _ := c.snapshotTexts(CaptureStreamCombined, 0)
	return lines
}

// Tail returns the last n retained combined lines.
func (c *Capture) Tail(n ...int) string {
	limit := 0
	if len(n) > 0 {
		limit = n[0]
	}
	lines, truncated := c.snapshotTexts(CaptureStreamCombined, limit)
	return joinCaptureLines(lines, truncated)
}

// Empty reports whether no lines have been retained.
func (c *Capture) Empty() bool {
	root := c.root()
	if root == nil {
		return true
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	return len(root.lines) == 0
}

// TaskName returns the owning task name when created via Task.Capture.
func (c *Capture) TaskName() string {
	root := c.root()
	if root == nil {
		return ""
	}
	return root.taskName
}

// DetailTail returns a ProblemOption attaching a user-visible presentation of
// the capture tail. Prefers stderr when separate streams were used.
func (c *Capture) DetailTail() ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		if text := c.detailText(); text != "" {
			p.Detail = text
		}
	})
}

// DetailTail free-function form (prefer method on Capture).
func DetailTail(c *Capture) ProblemOption {
	if c == nil {
		return problemOptionFunc(func(*Problem) {})
	}
	return c.DetailTail()
}

func (c *Capture) detailText() string {
	root := c.root()
	if root == nil {
		return ""
	}
	root.mu.Lock()
	defer root.mu.Unlock()

	prefer := CaptureStreamCombined
	if c.parent != nil {
		prefer = c.stream
	} else {
		// Prefer stderr when both streams have content.
		var hasOut, hasErr bool
		for _, ln := range root.lines {
			if ln.Stream == CaptureStreamStdout {
				hasOut = true
			}
			if ln.Stream == CaptureStreamStderr {
				hasErr = true
			}
		}
		if hasErr && hasOut {
			prefer = CaptureStreamStderr
		} else if hasErr {
			prefer = CaptureStreamStderr
		}
	}

	var texts []string
	for _, ln := range root.lines {
		switch prefer {
		case CaptureStreamStderr:
			if ln.Stream == CaptureStreamStderr {
				texts = append(texts, ln.Text)
			}
		case CaptureStreamStdout:
			if ln.Stream == CaptureStreamStdout {
				texts = append(texts, ln.Text)
			}
		default:
			texts = append(texts, ln.Text)
		}
	}
	// If filter emptied, fall back to all combined lines.
	if len(texts) == 0 {
		for _, ln := range root.lines {
			texts = append(texts, ln.Text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	var b strings.Builder
	if root.truncated {
		b.WriteString(truncationMarker)
		b.WriteByte('\n')
	}
	if len(texts) > 1 {
		fmt.Fprintf(&b, "Last %d lines:\n", len(texts))
	}
	b.WriteString(strings.Join(texts, "\n"))
	return b.String()
}

func (c *Capture) snapshotTexts(stream CaptureStream, limit int) ([]string, bool) {
	root := c.root()
	if root == nil {
		return nil, false
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	var texts []string
	for _, ln := range root.lines {
		if stream == CaptureStreamCombined || ln.Stream == stream {
			texts = append(texts, ln.Text)
		}
	}
	if limit > 0 && len(texts) > limit {
		texts = texts[len(texts)-limit:]
	}
	return append([]string(nil), texts...), root.truncated
}

func joinCaptureLines(lines []string, truncated bool) string {
	if len(lines) == 0 {
		return ""
	}
	if truncated {
		return truncationMarker + "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}

func (c *Capture) flushPendingLocked(stream CaptureStream) {
	buf := c.pendingFor(stream)
	line := buf.String()
	buf.Reset()
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = sanitize.Text(line)
	// Redact before ring retention so DetailTail and mirrors never expose secrets.
	if c.out != nil {
		line = c.out.redactString(line)
	}
	if len(line) > maxCaptureLineLen {
		line = line[:maxCaptureLineLen] + "…"
	}
	if line == "" {
		return
	}

	c.seq++
	c.lines = append(c.lines, capturedLine{Sequence: c.seq, Stream: stream, Text: line})
	c.nbytes += len(line) + 1
	for len(c.lines) > c.maxLines || c.nbytes > c.maxBytes {
		if len(c.lines) == 0 {
			break
		}
		c.truncated = true
		c.nbytes -= len(c.lines[0].Text) + 1
		c.lines = c.lines[1:]
	}

	if c.out == nil {
		return
	}
	if c.mirrorDiag || c.mirrorDebug {
		c.out.mirrorCaptureLine(c.mirrorDiag, c.mirrorDebug, c.taskName, line)
	}
}

// mirrorCaptureLine projects one capture line only when explicitly requested.
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
		interactive = live.IsInteractive() && !o.cfg.plain && !o.cfg.nonInteractive
	}
	o.mu.Unlock()
	if !allowDebug {
		return
	}
	// Only project when dual-stream diagnostics or interactive debug UI exist.
	if !hasDiag && !interactive {
		return
	}
	if taskName != "" {
		o.Debug(line, String("task", taskName))
	} else {
		o.Debug(line)
	}
}

var _ io.WriteCloser = (*Capture)(nil)
