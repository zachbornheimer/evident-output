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

// EvidenceStream identifies which process stream a line came from.
type EvidenceStream uint8

const (
	// EvidenceStreamCombined is Write() on the Evidence itself (merged by the runner).
	EvidenceStreamCombined EvidenceStream = iota
	// EvidenceStreamStdout is output.Stdout().
	EvidenceStreamStdout
	// EvidenceStreamStderr is output.Stderr().
	EvidenceStreamStderr
)

type capturedLine struct {
	Sequence uint64
	Stream   EvidenceStream
	Text     string
}

// Evidence is the retained/redacted process-output sink owned by a Task
// (preferred) or Output. "Stdout" would lie as a name — it also takes
// stderr and combined writes; Evidence says what it is for: durable,
// sanitized proof a failure can point back to.
//
//	upgrade := out.Task("brew packages")
//	proof := upgrade.Evidence() // silent retention by default
//	if err := run.Run(ctx, "brew", args, proof); err != nil {
//	    upgrade.Failf("brew upgrade failed: %w", err)
//	    return nil
//	}
//	upgrade.Done()
//
// Prefer task.Run for an *exec.Cmd — it wires Evidence and Phase together in
// one call. Reach for Evidence directly only when the caller already owns
// stdout/stderr plumbing (a custom runner, a non-exec.Cmd tool integration).
//
// Combined streams by default (P1): Write (merged), Stdout(), and Stderr() all
// feed the same bounded ring used by Text/Tail/DetailTail. Linters and most
// subprocess tools write diagnostics on stderr — route both streams into
// Evidence (or write the combined pipe into it directly) so failure evidence
// cannot escape the owning Task.
//
// Semantics:
//   - Always retains a bounded ring of sanitized lines (evidence exists even when
//     debug presentation is disabled).
//   - Default is silent: no Diagnostics/Debug mirror on success.
//   - Opt in with MirrorToDiagnostics / MirrorToDebug.
//   - Stdout/Stderr have independent pending buffers (no partial-line merge).
//   - DetailTail prefers stderr when separate streams were used, else combined.
type Evidence struct {
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
	stream EvidenceStream
	parent *Evidence
}

// Capture is Evidence's shipped v0.2.16 name.
//
// Deprecated: Use Evidence. Will be removed in v1.0.
type Capture = Evidence

// EvidenceOption configures Evidence (C9: finishes the Capture->Evidence
// rename in the option surface — CaptureOption is now a deprecated alias,
// deprecated.go).
type EvidenceOption interface {
	applyCapture(*Evidence)
}

type captureOptionFunc func(*Evidence)

func (f captureOptionFunc) applyCapture(c *Evidence) { f(c) }

// KeepLastLines sets how many trailing lines are retained (default 200).
func KeepLastLines(n int) EvidenceOption {
	return captureOptionFunc(func(c *Evidence) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

// MaxEvidenceBytes sets an approximate byte budget for retained lines
// (default 256KiB).
func MaxEvidenceBytes(n int) EvidenceOption {
	return captureOptionFunc(func(c *Evidence) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

// MirrorToDiagnostics copies each completed line to the Diagnostics writer.
// Default is off — Evidence retains proof without displaying it on success.
func MirrorToDiagnostics() EvidenceOption {
	return captureOptionFunc(func(c *Evidence) { c.mirrorDiag = true })
}

// MirrorToDebug journals each completed line via Debug when DebugLevel allows.
// Default is off.
func MirrorToDebug() EvidenceOption {
	return captureOptionFunc(func(c *Evidence) { c.mirrorDebug = true })
}

// Evidence returns the retained/redacted writer bound to this Task,
// get-or-create: the first call (from Evidence or PhaseWriter) allocates the
// ring and every later call returns that same instance, so evidence recorded
// through either path lands together and survives for DetailTail after Fail.
func (t *TaskHandle) Evidence(opts ...EvidenceOption) *Evidence {
	if t == nil || t.out == nil {
		return newEvidence(nil, "", "", opts...)
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return newEvidence(t.out, t.id, "", opts...)
	}
	if st.evidence == nil {
		st.evidence = newEvidence(t.out, t.id, st.name, opts...)
	}
	return st.evidence
}

// Capture is Evidence's shipped v0.2.16 name.
//
// Deprecated: Use TaskHandle.Evidence. Will be removed in v1.0.
func (t *TaskHandle) Capture(opts ...EvidenceOption) *Capture {
	return t.Evidence(opts...)
}

// Evidence returns a session-level retained/redacted writer with no owning
// Task. Prefer Task.Evidence so failure evidence attaches to an entity.
// Session-level Evidence is advanced; ordinary call sites should not use it.
func (o *Output) Evidence(opts ...EvidenceOption) *Evidence {
	return newEvidence(o, "", "", opts...)
}

// Capture is Evidence's shipped v0.2.16 name.
//
// Deprecated: Use Output.Evidence. Will be removed in v1.0.
func (o *Output) Capture(opts ...EvidenceOption) *Capture {
	return o.Evidence(opts...)
}

func newEvidence(out *Output, taskID, taskName string, opts ...EvidenceOption) *Evidence {
	c := &Evidence{
		out:         out,
		taskID:      taskID,
		taskName:    taskName,
		maxLines:    defaultCaptureLines,
		maxBytes:    defaultCaptureBytes,
		mirrorDiag:  false, // silent by default — release invariant
		mirrorDebug: false,
		stream:      EvidenceStreamCombined,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(c)
		}
	}
	return c
}

// Stdout returns a writer that records lines as stdout with its own pending buffer.
func (c *Evidence) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &Evidence{out: c.out, parent: c, stream: EvidenceStreamStdout}
}

// Stderr returns a writer that records lines as stderr with its own pending buffer.
func (c *Evidence) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &Evidence{out: c.out, parent: c, stream: EvidenceStreamStderr}
}

// Write implements io.Writer. Safe for concurrent use with Tail/DetailTail.
func (c *Evidence) Write(p []byte) (int, error) {
	root := c.root()
	if root == nil || root.out == nil {
		return len(p), nil
	}
	n := len(p)
	stream := EvidenceStreamCombined
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

// Close flushes trailing partial lines.
//
// On the root Capture (task.Capture()), every stream pending buffer is flushed
// so Stdout/Stderr partial lines are retained. On a side writer (Stdout/Stderr),
// only that stream is flushed.
func (c *Evidence) Close() error {
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
	root.flushIfPresentLocked(EvidenceStreamCombined)
	root.flushIfPresentLocked(EvidenceStreamStdout)
	root.flushIfPresentLocked(EvidenceStreamStderr)
	return nil
}

func (c *Evidence) flushIfPresentLocked(stream EvidenceStream) {
	if c.pendingFor(stream).Len() > 0 {
		c.flushPendingLocked(stream)
	}
}

func (c *Evidence) root() *Evidence {
	if c == nil {
		return nil
	}
	if c.parent != nil {
		return c.parent
	}
	return c
}

func (c *Evidence) pendingFor(stream EvidenceStream) *bytes.Buffer {
	switch stream {
	case EvidenceStreamStdout:
		return &c.pendingStdout
	case EvidenceStreamStderr:
		return &c.pendingStderr
	default:
		return &c.pendingCombined
	}
}

// Text returns all retained combined lines joined by newlines.
func (c *Evidence) Text() string {
	lines, truncated := c.snapshotTexts(EvidenceStreamCombined, 0)
	return joinCaptureLines(lines, truncated)
}

// Empty reports whether no completed lines and no pending fragments exist.
func (c *Evidence) Empty() bool {
	root := c.root()
	if root == nil {
		return true
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	if len(root.lines) > 0 {
		return false
	}
	return root.pendingFor(EvidenceStreamCombined).Len() == 0 &&
		root.pendingFor(EvidenceStreamStdout).Len() == 0 &&
		root.pendingFor(EvidenceStreamStderr).Len() == 0
}

// DetailTail returns a ProblemOption attaching a user-visible presentation of
// the capture tail. Prefers stderr when separate streams were used.
func (c *Evidence) DetailTail() ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		if text := c.detailText(); text != "" {
			p.Detail = text
		}
	})
}

func (c *Evidence) detailText() string {
	root := c.root()
	if root == nil {
		return ""
	}
	root.mu.Lock()
	defer root.mu.Unlock()

	prefer := EvidenceStreamCombined
	if c.parent != nil {
		prefer = c.stream
	} else {
		// Prefer stderr when both streams have content (including pending).
		hasOut := root.streamHasContentLocked(EvidenceStreamStdout)
		hasErr := root.streamHasContentLocked(EvidenceStreamStderr)
		if hasErr && hasOut {
			prefer = EvidenceStreamStderr
		} else if hasErr {
			prefer = EvidenceStreamStderr
		}
	}

	texts := root.textsForStreamLocked(prefer)
	// If filter emptied, fall back to all combined lines + all pendings.
	if len(texts) == 0 {
		texts = root.textsForStreamLocked(EvidenceStreamCombined)
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

func (c *Evidence) streamHasContentLocked(stream EvidenceStream) bool {
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
// EvidenceStreamCombined includes every stream's completed lines and all pendings.
// Pending fragments are snapshotted (not flushed) so concurrent Write stays safe.
func (c *Evidence) textsForStreamLocked(stream EvidenceStream) []string {
	var texts []string
	for _, ln := range c.lines {
		if stream == EvidenceStreamCombined || ln.Stream == stream {
			texts = append(texts, ln.Text)
		}
	}
	if stream == EvidenceStreamCombined {
		for _, s := range []EvidenceStream{EvidenceStreamCombined, EvidenceStreamStdout, EvidenceStreamStderr} {
			if p := c.pendingNormalizedLocked(s); p != "" {
				texts = append(texts, p)
			}
		}
	} else if p := c.pendingNormalizedLocked(stream); p != "" {
		texts = append(texts, p)
	}
	return texts
}

func (c *Evidence) snapshotTexts(stream EvidenceStream, limit int) ([]string, bool) {
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
func (c *Evidence) pendingNormalizedLocked(stream EvidenceStream) string {
	raw := c.pendingFor(stream).String()
	if raw == "" {
		return ""
	}
	return c.normalizeCaptureLine(raw)
}

func (c *Evidence) normalizeCaptureLine(line string) string {
	if !utf8.ValidString(line) {
		line = string(bytes.ToValidUTF8([]byte(line), []byte("\uFFFD")))
	}
	line = sanitize.Text(line)
	if c.out != nil {
		line = c.out.redactString(line)
	}
	return truncateUTF8(line, maxCaptureLineLen, "…")
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

func (c *Evidence) flushPendingLocked(stream EvidenceStream) {
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
		o.Debug(line, Field{Key: "task", Value: taskName})
	} else {
		o.Debug(line)
	}
}

var _ io.WriteCloser = (*Evidence)(nil)
