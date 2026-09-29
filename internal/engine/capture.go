package engine

import (
	"io"

	"github.com/zachbornheimer/evident-output/internal/engine/transcript"
)

// CaptureStream identifies which process stream a line came from.
type CaptureStream uint8

const (
	// CaptureStreamCombined is Write() on the capture itself (merged by the runner).
	CaptureStreamCombined CaptureStream = iota
	// CaptureStreamStdout is output.Stdout().
	CaptureStreamStdout
	// CaptureStreamStderr is output.Stderr().
	CaptureStreamStderr
)

// toTranscript maps the public vocabulary onto the transcript package's
// stream, kept separate so testdata/api_golden.txt never prints an internal
// import path.
func (s CaptureStream) toTranscript() transcript.Stream {
	switch s {
	case CaptureStreamStdout:
		return transcript.Stdout
	case CaptureStreamStderr:
		return transcript.Stderr
	default:
		return transcript.Combined
	}
}

// capture is the retained/redacted process-output sink owned by a Task
// (preferred) or Output. "Stdout" would lie as a name — it also takes
// stderr and combined writes; capture says what it is for: durable,
// sanitized process output a failure can point back to.
//
//	upgrade := out.Task("brew packages")
//	upgrade.Define(func(ctx context.Context) error {
//	    return run.Run(ctx, "brew", args, upgrade.capture())
//	})
//
// Prefer evo.Exec, or task.Writer() on an *exec.Cmd's Stdout/Stderr — both
// wire capture and Phase together. Reach for capture directly only when
// the caller already owns stdout/stderr plumbing (a custom runner, a
// non-exec.Cmd tool integration).
//
// Combined streams by default (P1): Write (merged), Stdout(), and Stderr() all
// feed the same bounded ring used by Text/Tail/DetailTail. Linters and most
// subprocess tools write diagnostics on stderr — route both streams into
// capture (or write the combined pipe into it directly) so failure output
// cannot escape the owning Task.
//
// Semantics:
//   - Always retains a bounded ring of sanitized lines (capture retains even when
//     debug presentation is disabled).
//   - Default is silent: no Diagnostics/Debug mirror on success.
//   - Opt in with MirrorToDiagnostics / MirrorToDebug.
//   - Stdout/Stderr have independent pending buffers (no partial-line merge).
//   - DetailTail prefers stderr when separate streams were used, else combined.
type capture struct {
	tr     *transcript.Transcript // nil when there is no Output: every write is discarded
	stream CaptureStream          // Combined on the root; Stdout/Stderr on a side writer
}

// CaptureOption configures a Capture.
type CaptureOption interface {
	applyCapture(*captureConfig)
}

type captureOptionFunc func(*captureConfig)

func (f captureOptionFunc) applyCapture(c *captureConfig) { f(c) }

// captureConfig collects CaptureOption settings before a capture's backing
// transcript is constructed.
type captureConfig struct {
	maxLines, maxBytes      int
	mirrorDiag, mirrorDebug bool
	onLine                  func(string)
}

// KeepLastLines sets how many trailing lines are retained (default 200).
func keepLastLines(n int) CaptureOption {
	return captureOptionFunc(func(c *captureConfig) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

// MaxCaptureBytes sets an approximate byte budget for retained lines
// (default 256KiB).
func maxCaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *captureConfig) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

// MirrorToDiagnostics copies each completed line to the Diagnostics writer.
// Default is off — capture retains output without displaying it on success.
func mirrorToDiagnostics() CaptureOption {
	return captureOptionFunc(func(c *captureConfig) { c.mirrorDiag = true })
}

// MirrorToDebug journals each completed line via Debug when DebugLevel allows.
// Default is off.
func mirrorToDebug() CaptureOption {
	return captureOptionFunc(func(c *captureConfig) { c.mirrorDebug = true })
}

// activityFeed reports each completed, sanitized/redacted line to fn (spec
// §23) — used only by Exec, which owns turning that line into the Task's
// current Doing activity. capture itself stays presentation-agnostic.
func activityFeed(fn func(text string)) CaptureOption {
	return captureOptionFunc(func(c *captureConfig) { c.onLine = fn })
}

// lineSink builds the transcript.Policy.OnLine callback: the activity feed
// first (if any), then the mirror projection (if either mirror is on).
// Returning nil when neither applies skips an empty call on every line.
func (c captureConfig) lineSink(out *Output, taskName string) func(string) {
	mirror := c.mirrorDiag || c.mirrorDebug
	if c.onLine == nil && !mirror {
		return nil
	}
	return func(line string) {
		if c.onLine != nil {
			c.onLine(line)
		}
		if mirror {
			out.mirrorCaptureLine(c.mirrorDiag, c.mirrorDebug, taskName, line)
		}
	}
}

// capture returns the retained/redacted writer bound to this Task,
// get-or-create: the first call (from capture or PhaseWriter) allocates the
// ring and every later call returns that same instance, so output recorded
// through either path lands together and survives for DetailTail after Fail.
func (t *TaskHandle) capture(opts ...CaptureOption) *capture {
	if t == nil || t.out == nil {
		return newCapture(nil, "", opts...)
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return newCapture(t.out, "", opts...)
	}
	if st.capture == nil {
		st.capture = newCapture(t.out, st.name, opts...)
	}
	return st.capture
}

// capture returns a session-level retained/redacted writer with no owning
// Task. Prefer task.Writer() so failure output attaches to an entity.
// Session-level capture is advanced; ordinary call sites should not use it.
func (o *Output) capture(opts ...CaptureOption) *capture {
	return newCapture(o, "", opts...)
}

func newCapture(out *Output, taskName string, opts ...CaptureOption) *capture {
	var cfg captureConfig
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(&cfg)
		}
	}
	if out == nil {
		return &capture{stream: CaptureStreamCombined}
	}
	return &capture{
		tr: transcript.New(transcript.Policy{
			MaxLines: cfg.maxLines,
			MaxBytes: cfg.maxBytes,
			Redact:   out.redactString,
			OnLine:   cfg.lineSink(out, taskName),
		}),
		stream: CaptureStreamCombined,
	}
}

// Stdout returns a writer that records lines as stdout with its own pending buffer.
func (c *capture) Stdout() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &capture{tr: c.tr, stream: CaptureStreamStdout}
}

// Stderr returns a writer that records lines as stderr with its own pending buffer.
func (c *capture) Stderr() io.Writer {
	if c == nil {
		return io.Discard
	}
	return &capture{tr: c.tr, stream: CaptureStreamStderr}
}

// Write implements io.Writer. Safe for concurrent use with Tail/DetailTail.
func (c *capture) Write(p []byte) (int, error) {
	if c == nil || c.tr == nil {
		return len(p), nil
	}
	c.tr.Write(c.stream.toTranscript(), p)
	return len(p), nil
}

// Close flushes trailing partial lines.
//
// On the root capture (task.capture()), every stream pending buffer is flushed
// so Stdout/Stderr partial lines are retained. On a side writer (Stdout/Stderr),
// only that stream is flushed.
func (c *capture) Close() error {
	if c == nil || c.tr == nil {
		return nil
	}
	if c.stream == CaptureStreamCombined {
		c.tr.FlushAll()
	} else {
		c.tr.Flush(c.stream.toTranscript())
	}
	return nil
}

// Text returns all retained combined lines joined by newlines.
func (c *capture) Text() string {
	if c == nil || c.tr == nil {
		return ""
	}
	return c.tr.Text()
}

// streamText returns one stream's retained lines joined by newlines, with
// no human-facing truncation marker — unlike Text/DetailTail, this feeds
// ExecResult.Stdout/Stderr, which a caller may parse as machine data
// (ZYS-850); truncated returns separately as ExecResult.Truncated instead
// of being prepended into the text.
func (c *capture) streamText(stream CaptureStream) string {
	if c == nil || c.tr == nil {
		return ""
	}
	return c.tr.StreamText(stream.toTranscript())
}

// wasTruncated reports whether the retained ring has ever dropped a line to
// stay within its bound (spec §8.4's ExecResult.Truncated) — one flag
// shared across streams because the bound itself is on total retained
// output, not per stream.
func (c *capture) wasTruncated() bool {
	if c == nil || c.tr == nil {
		return false
	}
	return c.tr.Truncated()
}

// Empty reports whether no completed lines and no pending fragments exist.
func (c *capture) Empty() bool {
	if c == nil || c.tr == nil {
		return true
	}
	return c.tr.Empty()
}

// DetailTail returns a ProblemOption attaching a user-visible presentation of
// the capture tail. Prefers stderr when separate streams were used. Sets
// Problem.CaptureTail rather than Problem.Detail: when the same Fail/Block
// call also carries an explicit Detail, that explicit text still renders (as
// the primary detail line) and this tail renders as an additional
// capture-tail line underneath, regardless of which option was passed first.
func (c *capture) DetailTail() ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		if text := c.detailText(); text != "" {
			p.CaptureTail = text
		}
	})
}

func (c *capture) detailText() string {
	if c == nil || c.tr == nil {
		return ""
	}
	return c.tr.Detail(c.stream.toTranscript())
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

var _ io.WriteCloser = (*capture)(nil)
