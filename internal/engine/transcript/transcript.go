// Package transcript is the retained/redacted process-output ring engine's
// capture wrapper delegates to. It is the one subpackage of internal/engine
// that owns a mutex: writes arrive from child-process copy goroutines that
// do not hold the engine's Output lock (process.go, phase_writer.go).
package transcript

import (
	"bytes"
	"sync"
	"unicode/utf8"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Default ring bounds (retained child process output, not a live UI).
const (
	defaultLines     = 200
	defaultBytes     = 256 << 10 // 256 KiB
	maxLineLen       = 4096
	truncationMarker = "[earlier output truncated]"
)

// Stream identifies which process stream a line came from.
type Stream uint8

const (
	// Combined is Write() on the transcript itself (merged by the runner).
	Combined Stream = iota
	// Stdout is output written through the Stdout view.
	Stdout
	// Stderr is output written through the Stderr view.
	Stderr
)

// Policy configures a Transcript.
type Policy struct {
	MaxLines int                 // <= 0: 200
	MaxBytes int                 // <= 0: 256 KiB
	Redact   func(string) string // nil: no redaction
	OnLine   func(string)        // nil: none. Runs under the Transcript's lock, after retention.
}

type line struct {
	Sequence uint64
	Stream   Stream
	Text     string
}

// Transcript is a bounded, sanitized ring of process-output lines with
// independent per-stream pending buffers for partial-line assembly.
type Transcript struct {
	mu sync.Mutex

	// emit serializes OnLine calls and keeps them outside mu: flush
	// retains a line under mu, then hands over to emit before calling
	// OnLine, so callback order still matches retention order but a
	// callback that re-enters the owning engine (e.g. to mirror the line)
	// never has to wait on a lock resolve already holds while it, in
	// turn, waits here (§4 C10).
	emit sync.Mutex

	pending [3]bytes.Buffer // indexed by Stream

	lines     []line
	seq       uint64
	maxLines  int
	maxBytes  int
	nbytes    int
	truncated bool

	redact func(string) string
	onLine func(string)
}

// New constructs a ready Transcript from p.
func New(p Policy) *Transcript {
	maxLines := p.MaxLines
	if maxLines <= 0 {
		maxLines = defaultLines
	}
	maxBytes := p.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultBytes
	}
	return &Transcript{
		maxLines: maxLines,
		maxBytes: maxBytes,
		redact:   p.Redact,
		onLine:   p.OnLine,
	}
}

func (t *Transcript) pendingFor(s Stream) *bytes.Buffer {
	return &t.pending[s]
}

// Write records p as stream s, splitting on newlines and flushing each
// completed line into the retained ring.
func (t *Transcript) Write(s Stream, p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	buf := t.pendingFor(s)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			buf.Write(p)
			break
		}
		buf.Write(p[:i])
		t.flushLocked(s)
		buf = t.pendingFor(s) // flushLocked released and re-acquired mu
		p = p[i+1:]
	}
	if buf.Len() > maxLineLen*2 {
		t.flushLocked(s)
	}
}

// Flush flushes stream s's trailing partial line, if any.
func (t *Transcript) Flush(s Stream) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pendingFor(s).Len() > 0 {
		t.flushLocked(s)
	}
}

// FlushAll flushes every stream's trailing partial line, Combined then
// Stdout then Stderr.
func (t *Transcript) FlushAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range []Stream{Combined, Stdout, Stderr} {
		if t.pendingFor(s).Len() > 0 {
			t.flushLocked(s)
		}
	}
}

func (t *Transcript) normalizeLine(raw string) string {
	if !utf8.ValidString(raw) {
		raw = string(bytes.ToValidUTF8([]byte(raw), []byte("�")))
	}
	raw = txt.Text(raw)
	if t.redact != nil {
		raw = t.redact(raw)
	}
	return txt.TruncateUTF8(raw, maxLineLen, "…")
}

// flushLocked must be called with mu held and returns with mu held. When
// OnLine is set, it hands over from mu to emit around the callback: the
// line is retained under mu first, then emit is taken and mu released
// before OnLine runs, so a callback that re-enters the owning engine never
// waits on mu while this goroutine holds it. emit still serializes
// callbacks in retention order (§4 C10).
func (t *Transcript) flushLocked(s Stream) {
	buf := t.pendingFor(s)
	raw := buf.String()
	buf.Reset()
	text := t.normalizeLine(raw)
	if text == "" {
		return
	}

	t.seq++
	t.lines = append(t.lines, line{Sequence: t.seq, Stream: s, Text: text})
	t.nbytes += len(text) + 1
	for len(t.lines) > t.maxLines || t.nbytes > t.maxBytes {
		if len(t.lines) == 0 {
			break
		}
		t.truncated = true
		t.nbytes -= len(t.lines[0].Text) + 1
		t.lines = t.lines[1:]
	}

	onLine := t.onLine
	if onLine == nil {
		return
	}
	t.emit.Lock()
	t.mu.Unlock()
	onLine(text)
	t.emit.Unlock()
	t.mu.Lock()
}
