package evo

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Default ring bounds for Capture (child process evidence, not a live UI).
const (
	defaultCaptureLines = 48
	defaultCaptureBytes = 16 * 1024
	maxCaptureLineLen   = 4096
)

// Capture is an Output-owned sink for external process stdout/stderr.
//
// It is the elegant alternative to hand-threading DebugWriter into every Run:
//
//	cap := out.Capture()
//	err := run.Run(ctx, "brew", args, cap)
//	if err != nil {
//	    task.Fail("brew upgrade failed", evo.Cause(err), evo.DetailTail(cap))
//	}
//
// Semantics:
//   - Never writes into the live region or progressive human Items/Tasks stream.
//   - Keeps a bounded ring of sanitized lines for Fail/Block Detail (Tail).
//   - When Diagnostics is configured, mirrors each line there (agent/LaunchAgent logs).
//   - Does not use Debug() / DebugLevel — child chatter is evidence, not a log level.
//
// Application code still owns process execution (cmdrun/exec). Capture only owns
// where that chatter goes for presentation and postmortem.
type Capture struct {
	out *Output

	mu       sync.Mutex
	buf      bytes.Buffer
	lines    []string
	maxLines int
	maxBytes int
	nbytes   int
	// mirrorDiag copies each completed line to the Diagnostics writer when set.
	mirrorDiag bool
}

// CaptureOption configures Capture.
type CaptureOption interface {
	applyCapture(*Capture)
}

type captureOptionFunc func(*Capture)

func (f captureOptionFunc) applyCapture(c *Capture) { f(c) }

// CaptureLines sets how many trailing lines Tail retains (default 48).
func CaptureLines(n int) CaptureOption {
	return captureOptionFunc(func(c *Capture) {
		if n > 0 {
			c.maxLines = n
		}
	})
}

// CaptureBytes sets an approximate byte budget for retained lines (default 16KiB).
func CaptureBytes(n int) CaptureOption {
	return captureOptionFunc(func(c *Capture) {
		if n > 0 {
			c.maxBytes = n
		}
	})
}

// CaptureQuiet disables mirroring lines to Diagnostics (buffer/Tail only).
func CaptureQuiet() CaptureOption {
	return captureOptionFunc(func(c *Capture) { c.mirrorDiag = false })
}

// Capture returns a process-output sink bound to this Output.
// Prefer this over DebugWriter for child commands.
func (o *Output) Capture(opts ...CaptureOption) *Capture {
	c := &Capture{
		out:        o,
		maxLines:   defaultCaptureLines,
		maxBytes:   defaultCaptureBytes,
		mirrorDiag: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCapture(c)
		}
	}
	return c
}

// Write implements io.Writer. Safe for concurrent use with Tail.
func (c *Capture) Write(p []byte) (int, error) {
	if c == nil || c.out == nil {
		return len(p), nil
	}
	n := len(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			c.buf.Write(p)
			break
		}
		c.buf.Write(p[:i])
		c.flushLineLocked()
		p = p[i+1:]
	}
	if c.buf.Len() > maxCaptureLineLen*2 {
		c.flushLineLocked()
	}
	return n, nil
}

// Close flushes a trailing partial line. Idempotent for use with defer.
func (c *Capture) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buf.Len() > 0 {
		c.flushLineLocked()
	}
	return nil
}

// Tail returns the retained trailing lines joined by newlines (no trailing NL).
// Empty when the process produced no captured text.
func (c *Capture) Tail() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.lines) == 0 {
		return ""
	}
	return strings.Join(c.lines, "\n")
}

// Empty reports whether no lines have been retained.
func (c *Capture) Empty() bool {
	return c.Tail() == ""
}

func (c *Capture) flushLineLocked() {
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
	c.nbytes += len(line) + 1
	for len(c.lines) > c.maxLines || c.nbytes > c.maxBytes {
		if len(c.lines) == 0 {
			break
		}
		c.nbytes -= len(c.lines[0]) + 1
		c.lines = c.lines[1:]
	}
	if c.mirrorDiag {
		// Unlock-free path: Output takes its own lock.
		// Mirror outside Capture.mu to avoid holding two locks in a fixed order issue
		// when Debug also takes Output.mu — we release Capture.mu after loop.
		// Call mirror after unlock in Write — do it here with re-entrant care:
		// writeDiagnosticText takes Output.mu only.
		c.out.writeDiagnosticText(line + "\n")
	}
}

// DetailTail attaches Capture.Tail as user-visible Detail when non-empty.
// Use with Fail/Block/Warn:
//
//	task.Fail("brew failed", evo.Cause(err), evo.DetailTail(cap))
func DetailTail(c *Capture) ProblemOption {
	return problemOptionFunc(func(p *Problem) {
		if c == nil {
			return
		}
		if t := c.Tail(); t != "" {
			p.Detail = t
		}
	})
}

// Ensure Capture is usable as cmd.Stdout/Stderr and with defer Close.
var _ io.WriteCloser = (*Capture)(nil)
