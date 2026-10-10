package engine

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/record"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Default ring bounds for Capture (child process evidence, not a live UI).
const (
	defaultCaptureLines = 200
	defaultCaptureBytes = 256 << 10 // 256 KiB
	maxCaptureLineLen   = 4096
	// maxCaptureChunk is the most raw bytes one retained line is built
	// from: a pending partial line is flushed past it, and normalization
	// reads no further, so a flood without newlines costs work in
	// proportion to what is kept rather than to what the child wrote.
	maxCaptureChunk  = maxCaptureLineLen * 2
	truncationMarker = "[earlier output truncated]"
)

// CaptureStream identifies which process stream a line came from.
type CaptureStream uint8

const (
	// CaptureStreamCombined is Write() on the evidence itself (merged by the runner).
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

// evidence is the retained/redacted process-output sink owned by a Task
// (preferred) or Output. "Stdout" would lie as a name — it also takes
// stderr and combined writes; evidence says what it is for: durable,
// sanitized proof a failure can point back to.
//
//	upgrade := out.Task("brew packages")
//	upgrade.Define(func(ctx context.Context) error {
//	    return run.Run(ctx, "brew", args, upgrade.Capture())
//	})
//
// Prefer evo.Exec, or task.Writer() on an *exec.Cmd's Stdout/Stderr — both
// wire evidence and Phase together. Reach for Capture directly only when
// the caller already owns stdout/stderr plumbing (a custom runner, a
// non-exec.Cmd tool integration).
//
// Combined streams by default (P1): Write (merged), Stdout(), and Stderr() all
// feed the same bounded ring used by Text/Tail/DetailTail. Linters and most
// subprocess tools write diagnostics on stderr — route both streams into
// evidence (or write the combined pipe into it directly) so failure evidence
// cannot escape the owning Task.
//
// Semantics:
//   - Always retains a bounded ring of sanitized lines (evidence exists even when
//     debug presentation is disabled).
//   - Default is silent: no Diagnostics/Debug mirror on success.
//   - Opt in with MirrorToDiagnostics / MirrorToDebug.
//   - Stdout/Stderr have independent pending buffers (no partial-line merge).
//   - DetailTail prefers stderr when separate streams were used, else combined.
type evidence struct {
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
	parent *evidence

	// onLine, when set, receives each completed line's sanitized/redacted
	// text (spec §23: "each complete non-empty line becomes the task's
	// current activity") — Exec wires it to TaskHandle.Doing so a running
	// child's own output narrates progress without evidence needing to know
	// what a Task is.
	onLine func(text string)
}

// CaptureOption configures Capture.
type CaptureOption interface {
	applyCapture(*evidence)
}

type captureOptionFunc func(*evidence)

func (f captureOptionFunc) applyCapture(c *evidence) { f(c) }

// keepLastLines sets how many trailing lines the ring retains (default 200).
// KeepLastLines was removed in 1.1; MaxCaptureBytes is the public option.
func keepLastLines(n int) CaptureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

// MaxCaptureBytes sets an approximate byte budget for retained lines
// (default 256KiB).
func maxCaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *evidence) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

// MirrorToDiagnostics copies each completed line to the Diagnostics writer.
// Default is off — evidence retains proof without displaying it on success.
func mirrorToDiagnostics() CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDiag = true })
}

// MirrorToDebug journals each completed line via Debug when DebugLevel allows.
// Default is off.
func mirrorToDebug() CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.mirrorDebug = true })
}

// activityFeed reports each completed, sanitized/redacted line to fn (spec
// §23) — used only by Exec, which owns turning that line into the Task's
// current Doing activity. evidence itself stays presentation-agnostic.
func activityFeed(fn func(text string)) CaptureOption {
	return captureOptionFunc(func(c *evidence) { c.onLine = fn })
}

// evidence returns the retained/redacted writer bound to this Task,
// get-or-create: the first call (from Capture or PhaseWriter) allocates the
// ring and every later call returns that same instance, so evidence recorded
// through either path lands together and survives for DetailTail after Fail.
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

// evidence returns a session-level retained/redacted writer with no owning
// Task. Prefer Task.Capture so failure evidence attaches to an entity.
// Session-level evidence is advanced; ordinary call sites should not use it.
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
func (c *evidence) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: CaptureStreamStdout}
}

// Stderr returns a writer that records lines as stderr with its own pending buffer.
func (c *evidence) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &evidence{out: c.out, parent: c, stream: CaptureStreamStderr}
}

// Write implements io.Writer. Safe for concurrent use with Tail/DetailTail.
func (c *evidence) Write(p []byte) (int, error) {
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
	if buf.Len() > maxCaptureChunk {
		root.flushPendingLocked(stream)
	}
	return n, nil
}

// Close flushes trailing partial lines.
//
// On the root evidence (task.Capture()), every stream pending buffer is flushed
// so Stdout/Stderr partial lines are retained. On a side writer (Stdout/Stderr),
// only that stream is flushed.
func (c *evidence) Close() error {
	root := c.root()
	if root == nil {
		return nil
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	if c.parent != nil {
		root.flushIfPresentLocked(c.stream)
		return nil
	}
	root.flushIfPresentLocked(CaptureStreamCombined)
	root.flushIfPresentLocked(CaptureStreamStdout)
	root.flushIfPresentLocked(CaptureStreamStderr)
	return nil
}

func (c *evidence) flushIfPresentLocked(stream CaptureStream) {
	if c.pendingFor(stream).Len() > 0 {
		c.flushPendingLocked(stream)
	}
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

func (c *evidence) pendingFor(stream CaptureStream) *bytes.Buffer {
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
func (c *evidence) Text() string {
	lines, truncated := c.snapshotTexts(CaptureStreamCombined, 0)
	return joinCaptureLines(lines, truncated)
}

// streamText returns one stream's retained lines joined by newlines, with
// no human-facing truncation marker — unlike Text/DetailTail, this feeds
// ExecResult.Stdout/Stderr, which a caller may parse as machine data
// (ZYS-850); truncated returns separately as ExecResult.Truncated instead
// of being prepended into the text.
func (c *evidence) streamText(stream CaptureStream) string {
	lines, _ := c.snapshotTexts(stream, 0)
	return strings.Join(lines, "\n")
}

// wasTruncated reports whether the retained ring has ever dropped a line to
// stay within its bound (spec §8.4's ExecResult.Truncated) — one flag
// shared across streams because the bound itself is on total retained
// evidence, not per stream.
func (c *evidence) wasTruncated() bool {
	root := c.root()
	if root == nil {
		return false
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	return root.truncated
}

// Empty reports whether no completed lines and no pending fragments exist.
func (c *evidence) Empty() bool {
	root := c.root()
	if root == nil {
		return true
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	if len(root.lines) > 0 {
		return false
	}
	return root.pendingFor(CaptureStreamCombined).Len() == 0 &&
		root.pendingFor(CaptureStreamStdout).Len() == 0 &&
		root.pendingFor(CaptureStreamStderr).Len() == 0
}

// DetailTail returns a ProblemOption attaching a user-visible presentation of
// the capture tail. Prefers stderr when separate streams were used. Sets
// Problem.EvidenceTail rather than Problem.Detail: when the same Fail/Block
// call also carries an explicit Detail, that explicit text still renders (as
// the primary detail line) and this tail renders as an additional evidence
// line underneath, regardless of which option was passed first.
func (c *evidence) DetailTail() ProblemOption {
	return record.WithEvidenceTail(c.detailText)
}

func (c *evidence) detailText() string {
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
		// Prefer stderr when both streams have content (including pending).
		hasOut := root.streamHasContentLocked(CaptureStreamStdout)
		hasErr := root.streamHasContentLocked(CaptureStreamStderr)
		if hasErr && hasOut {
			prefer = CaptureStreamStderr
		} else if hasErr {
			prefer = CaptureStreamStderr
		}
	}

	texts := root.textsForStreamLocked(prefer)
	// If filter emptied, fall back to all combined lines + all pendings.
	if len(texts) == 0 {
		texts = root.textsForStreamLocked(CaptureStreamCombined)
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

func (c *evidence) streamHasContentLocked(stream CaptureStream) bool {
	if c.pendingFor(stream).Len() > 0 {
		return true
	}
	for _, ln := range c.lines {
		if ln.Stream == stream {
			return true
		}
	}
	return false
}

// textsForStreamLocked returns completed lines plus pending fragments for stream.
// CaptureStreamCombined includes every stream's completed lines and all pendings.
// Pending fragments are snapshotted (not flushed) so concurrent Write stays safe.
func (c *evidence) textsForStreamLocked(stream CaptureStream) []string {
	var texts []string
	for _, ln := range c.lines {
		if stream == CaptureStreamCombined || ln.Stream == stream {
			texts = append(texts, ln.Text)
		}
	}
	if stream == CaptureStreamCombined {
		for _, s := range []CaptureStream{CaptureStreamCombined, CaptureStreamStdout, CaptureStreamStderr} {
			if p := c.pendingNormalizedLocked(s); p != "" {
				texts = append(texts, p)
			}
		}
	} else if p := c.pendingNormalizedLocked(stream); p != "" {
		texts = append(texts, p)
	}
	return texts
}

func (c *evidence) snapshotTexts(stream CaptureStream, limit int) ([]string, bool) {
	root := c.root()
	if root == nil {
		return nil, false
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	texts := root.textsForStreamLocked(stream)
	if limit > 0 && len(texts) > limit {
		texts = texts[len(texts)-limit:]
	}
	return append([]string(nil), texts...), root.truncated
}

// pendingNormalizedLocked returns a sanitized/redacted view of a pending buffer
// without flushing it into the ring.
func (c *evidence) pendingNormalizedLocked(stream CaptureStream) string {
	raw := c.pendingFor(stream).String()
	if raw == "" {
		return ""
	}
	return c.normalizeCaptureLine(raw)
}

func (c *evidence) normalizeCaptureLine(line string) string {
	cut := len(line) > maxCaptureChunk
	if cut {
		line = line[:maxCaptureChunk]
	}
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = txt.Text(line)
	if c.out != nil {
		line = c.out.redactString(line)
	}
	line = txt.TruncateUTF8(line, maxCaptureLineLen, "…")
	if cut && !strings.HasSuffix(line, "…") {
		line += "…"
	}
	return line
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

func (c *evidence) flushPendingLocked(stream CaptureStream) {
	buf := c.pendingFor(stream)
	line := buf.String()
	buf.Reset()
	line = c.normalizeCaptureLine(line)
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

	if c.onLine != nil {
		c.onLine(line)
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
		interactive = live.IsInteractive() && !o.cfg.plain
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
		o.debug(line, Field{Key: "task", Value: taskName})
	} else {
		o.debug(line)
	}
}

var _ io.WriteCloser = (*evidence)(nil)
