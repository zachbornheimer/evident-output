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
// Tail of output usually holds the actionable failure.
const (
	defaultCaptureLines = 200
	defaultCaptureBytes = 256 << 10 // 256 KiB
	maxCaptureLineLen   = 4096
	truncationMarker    = "[earlier output truncated]"
)

type captureStreamKind int

const (
	streamCombined captureStreamKind = iota
	streamStdout
	streamStderr
)

// Capture is a process-output sink owned by a Task (preferred) or Output.
//
// Recommended:
//
//	upgrade := out.Task("brew packages")
//	output := upgrade.Capture()
//	if err := run.Run(ctx, "brew", args, output); err != nil {
//	    upgrade.Fail("brew upgrade failed", evo.Cause(err), output.DetailTail())
//	    return nil
//	}
//	upgrade.Done()
//
// Semantics:
//   - Always retains a bounded ring of sanitized lines (evidence exists even when
//     debug presentation is disabled).
//   - Implements io.Writer; concurrency-safe.
//   - Never paints the live Items/Tasks region; never auto-surfaces on success.
//   - Mirrors to Diagnostics when configured; mirrors to Debug journal when
//     DebugLevel allows (task-labeled).
//   - DetailTail is user-visible failure evidence; Cause is structured diagnostic.
//
// Application code still owns process execution. Capture only owns presentation evidence.
type Capture struct {
	out      *Output
	taskID   string
	taskName string

	mu         sync.Mutex
	buf        bytes.Buffer
	lines      []string // combined (Write, or stdout+stderr merge order)
	stdout     []string
	stderr     []string
	maxLines   int
	maxBytes   int
	nbytes     int
	truncated  bool
	mirrorDiag bool
	// stream is set only on side writers returned by Stdout/Stderr.
	stream captureStreamKind
	parent *Capture // non-nil for Stdout()/Stderr() side writers
}

// CaptureOption configures Capture.
type CaptureOption interface {
	applyCapture(*Capture)
}

type captureOptionFunc func(*Capture)

func (f captureOptionFunc) applyCapture(c *Capture) { f(c) }

// KeepLastLines sets how many trailing lines are retained (default 200).
// Alias of CaptureLines for the designer-facing name.
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
// Alias of CaptureBytes.
func MaxCaptureBytes(n int) CaptureOption { return CaptureBytes(n) }

// CaptureBytes sets an approximate byte budget for retained lines (default 256KiB).
func CaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *Capture) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

// CaptureQuiet disables Diagnostics and Debug mirrors (buffer/Tail only).
func CaptureQuiet() CaptureOption {
	return captureOptionFunc(func(c *Capture) { c.mirrorDiag = false })
}

// Capture returns a process-output sink bound to this Task.
// The capture is associated with the task for debug labeling and failure detail.
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
		out:        out,
		taskID:     taskID,
		taskName:   taskName,
		maxLines:   defaultCaptureLines,
		maxBytes:   defaultCaptureBytes,
		mirrorDiag: true,
		stream:     streamCombined,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(c)
		}
	}
	return c
}

// Stdout returns a writer that records lines as stdout (and into the combined ring).
// Use when the runner supports separate streams:
//
//	cmd.Stdout = output.Stdout()
//	cmd.Stderr = output.Stderr()
func (c *Capture) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &Capture{out: c.out, parent: c, stream: streamStdout}
}

// Stderr returns a writer that records lines as stderr (and into the combined ring).
func (c *Capture) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &Capture{out: c.out, parent: c, stream: streamStderr}
}

// Write implements io.Writer. Safe for concurrent use with Tail/DetailTail.
func (c *Capture) Write(p []byte) (int, error) {
	root := c.root()
	if root == nil || root.out == nil {
		return len(p), nil
	}
	n := len(p)
	root.mu.Lock()
	defer root.mu.Unlock()
	stream := streamCombined
	if c.parent != nil {
		stream = c.stream
	} else {
		stream = c.stream
	}
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			root.buf.Write(p)
			break
		}
		root.buf.Write(p[:i])
		root.flushLineLocked(stream)
		p = p[i+1:]
	}
	if root.buf.Len() > maxCaptureLineLen*2 {
		root.flushLineLocked(stream)
	}
	return n, nil
}

// Close flushes a trailing partial line. Idempotent for use with defer.
func (c *Capture) Close() error {
	root := c.root()
	if root == nil {
		return nil
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	if root.buf.Len() > 0 {
		stream := streamCombined
		if c.parent != nil {
			stream = c.stream
		}
		root.flushLineLocked(stream)
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

// Text returns all retained combined lines joined by newlines.
func (c *Capture) Text() string {
	lines, truncated := c.snapshotLines(streamCombined, 0)
	return c.joinLines(lines, truncated)
}

// Lines returns a copy of retained combined lines (oldest first).
func (c *Capture) Lines() []string {
	lines, _ := c.snapshotLines(streamCombined, 0)
	return lines
}

// Tail returns the last n retained combined lines joined by newlines.
// If n <= 0, returns the full retained ring (same as Text).
func (c *Capture) Tail(n ...int) string {
	limit := 0
	if len(n) > 0 {
		limit = n[0]
	}
	lines, truncated := c.snapshotLines(streamCombined, limit)
	return c.joinLines(lines, truncated)
}

// Empty reports whether no combined lines have been retained.
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

// DetailTail returns a ProblemOption that attaches a user-visible presentation of
// the capture tail. Prefer stderr content when separate streams were used.
// Does not mutate the task; compose with Fail/Block/Warn:
//
//	upgrade.Fail("brew upgrade failed", evo.Cause(err), output.DetailTail())
func (c *Capture) DetailTail() ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		if text := c.detailText(); text != "" {
			p.Detail = text
		}
	})
}

// DetailTail is a free-function form of Capture.DetailTail for older call sites.
// Prefer output.DetailTail() on the capture value.
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

	// Prefer stderr when the caller used Stderr() and it has content.
	var lines []string
	truncated := root.truncated
	switch {
	case c.parent != nil && c.stream == streamStderr && len(root.stderr) > 0:
		lines = append([]string(nil), root.stderr...)
	case c.parent != nil && c.stream == streamStdout && len(root.stdout) > 0:
		lines = append([]string(nil), root.stdout...)
	case len(root.stderr) > 0 && len(root.stdout) > 0:
		// Combined capture path used both streams separately: prefer stderr for failure.
		lines = append([]string(nil), root.stderr...)
	default:
		lines = append([]string(nil), root.lines...)
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	if truncated {
		b.WriteString(truncationMarker)
		b.WriteByte('\n')
	}
	if len(lines) > 1 {
		fmt.Fprintf(&b, "Last %d lines:\n", len(lines))
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

func (c *Capture) snapshotLines(stream captureStreamKind, limit int) ([]string, bool) {
	root := c.root()
	if root == nil {
		return nil, false
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	var src []string
	switch stream {
	case streamStdout:
		src = root.stdout
	case streamStderr:
		src = root.stderr
	default:
		src = root.lines
	}
	if limit > 0 && len(src) > limit {
		src = src[len(src)-limit:]
	}
	out := append([]string(nil), src...)
	return out, root.truncated
}

func (c *Capture) joinLines(lines []string, truncated bool) string {
	if len(lines) == 0 {
		return ""
	}
	if truncated {
		return truncationMarker + "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}

func (c *Capture) flushLineLocked(stream captureStreamKind) {
	line := c.buf.String()
	c.buf.Reset()
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = sanitize.Text(line)
	if len(line) > maxCaptureLineLen {
		line = line[:maxCaptureLineLen] + "…"
	}
	if line == "" {
		return
	}

	c.lines = append(c.lines, line)
	switch stream {
	case streamStdout:
		c.stdout = append(c.stdout, line)
	case streamStderr:
		c.stderr = append(c.stderr, line)
	}

	c.nbytes += len(line) + 1
	for len(c.lines) > c.maxLines || c.nbytes > c.maxBytes {
		if len(c.lines) == 0 {
			break
		}
		c.truncated = true
		c.nbytes -= len(c.lines[0]) + 1
		c.lines = c.lines[1:]
	}
	// Bound per-stream rings to the same maxLines (keep newest).
	for len(c.stdout) > c.maxLines {
		c.stdout = c.stdout[1:]
		c.truncated = true
	}
	for len(c.stderr) > c.maxLines {
		c.stderr = c.stderr[1:]
		c.truncated = true
	}

	// Presentation mirrors (evidence already retained in the ring).
	// Lock order: Capture.mu → Output.mu (Debug / Diagnostics); never reverse.
	if c.out == nil {
		return
	}
	c.out.mirrorCaptureLine(c.mirrorDiag, c.taskName, line)
}

// mirrorCaptureLine projects one capture line without losing ring evidence.
// DebugLevel controls presentation only; the ring is independent.
func (o *Output) mirrorCaptureLine(mirrorDiag bool, taskName, line string) {
	if o == nil {
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

	// Prefer Debug (task-labeled) when enabled and there is a safe projection target
	// (Diagnostics dual-stream or interactive debug UI). Never dump child chatter onto
	// a solo human primary stream.
	if allowDebug && (hasDiag || interactive) {
		if taskName != "" {
			o.Debug(line, String("task", taskName))
		} else {
			o.Debug(line)
		}
		return
	}
	if mirrorDiag && hasDiag {
		o.writeDiagnosticText(line + "\n")
	}
}

// Ensure Capture is usable as cmd.Stdout/Stderr and with defer Close.
var _ io.WriteCloser = (*Capture)(nil)
