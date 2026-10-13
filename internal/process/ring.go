package process

import (
	"bytes"
	"strings"
	"sync"
)

// Default ring bounds for child-process evidence (not a live UI).
const (
	DefaultCaptureLines = 200
	DefaultCaptureBytes = 256 << 10 // 256 KiB
	// MaxCaptureChunk is the most raw bytes one retained line is built from.
	MaxCaptureChunk = 4096 * 2
)

// TruncationMarker is prepended to Text when the ring has dropped lines.
const TruncationMarker = "[earlier output truncated]"

// Stream identifies which process stream a retained line came from.
type Stream uint8

const (
	StreamCombined Stream = iota
	StreamStdout
	StreamStderr
)

// Line is one retained, already-normalized evidence line.
type Line struct {
	Sequence uint64
	Stream   Stream
	Text     string
}

// Ring is the bounded capture buffer: pending fragments per stream and a
// sequenced combined tail. Callers sanitize and redact before Append; the
// ring only bounds what is kept.
type Ring struct {
	mu sync.Mutex

	pendingCombined bytes.Buffer
	pendingStdout   bytes.Buffer
	pendingStderr   bytes.Buffer

	lines     []Line
	seq       uint64
	maxLines  int
	maxBytes  int
	nbytes    int
	truncated bool
	onAppend  func(text string)
}

// NewRing is an empty ring with the given line and byte bounds. Non-positive
// values fall back to the defaults.
func NewRing(maxLines, maxBytes int) *Ring {
	if maxLines <= 0 {
		maxLines = DefaultCaptureLines
	}
	if maxBytes <= 0 {
		maxBytes = DefaultCaptureBytes
	}
	return &Ring{maxLines: maxLines, maxBytes: maxBytes}
}

func (r *Ring) pending(stream Stream) *bytes.Buffer {
	switch stream {
	case StreamStdout:
		return &r.pendingStdout
	case StreamStderr:
		return &r.pendingStderr
	default:
		return &r.pendingCombined
	}
}

// Write records p on stream. normalize is applied to each completed line
// (and to a fragment flushed because it exceeded the chunk bound).
func (r *Ring) Write(p []byte, stream Stream, normalize func(string) string) (int, error) {
	if r == nil {
		return len(p), nil
	}
	n := len(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := r.pending(stream)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			buf.Write(p)
			break
		}
		buf.Write(p[:i])
		r.flushLocked(stream, normalize)
		p = p[i+1:]
	}
	if buf.Len() > MaxCaptureChunk {
		r.flushLocked(stream, normalize)
	}
	return n, nil
}

// Close flushes trailing partial lines on the given streams.
func (r *Ring) Close(streams []Stream, normalize func(string) string) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, stream := range streams {
		if r.pending(stream).Len() > 0 {
			r.flushLocked(stream, normalize)
		}
	}
	return nil
}

func (r *Ring) flushLocked(stream Stream, normalize func(string) string) {
	buf := r.pending(stream)
	line := buf.String()
	buf.Reset()
	if normalize != nil {
		line = normalize(line)
	}
	if line == "" {
		return
	}
	r.seq++
	r.lines = append(r.lines, Line{Sequence: r.seq, Stream: stream, Text: line})
	r.nbytes += len(line) + 1
	for len(r.lines) > r.maxLines || r.nbytes > r.maxBytes {
		if len(r.lines) == 0 {
			break
		}
		r.truncated = true
		r.nbytes -= len(r.lines[0].Text) + 1
		r.lines = r.lines[1:]
	}
	if r.onAppend != nil {
		r.onAppend(line)
	}
}

// SetOnAppend reports each completed retained line. Must be set before Write.
func (r *Ring) SetOnAppend(fn func(text string)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onAppend = fn
}

// Len is the number of completed lines currently retained.
func (r *Ring) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lines)
}

// Truncated reports whether a line has ever been dropped to stay in bound.
func (r *Ring) Truncated() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.truncated
}

// Empty reports whether no completed lines and no pending fragments exist.
func (r *Ring) Empty() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) > 0 {
		return false
	}
	return r.pendingCombined.Len() == 0 && r.pendingStdout.Len() == 0 && r.pendingStderr.Len() == 0
}

// Texts is completed lines plus pending fragments for stream. Combined
// includes every stream. limit <= 0 keeps them all. The returned slice is
// a copy.
func (r *Ring) Texts(stream Stream, limit int, normalize func(string) string) (lines []string, truncated bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	texts := r.textsLocked(stream, normalize)
	if limit > 0 && len(texts) > limit {
		texts = texts[len(texts)-limit:]
	}
	return append([]string(nil), texts...), r.truncated
}

func (r *Ring) textsLocked(stream Stream, normalize func(string) string) []string {
	var texts []string
	for _, ln := range r.lines {
		if stream == StreamCombined || ln.Stream == stream {
			texts = append(texts, ln.Text)
		}
	}
	if stream == StreamCombined {
		for _, s := range []Stream{StreamCombined, StreamStdout, StreamStderr} {
			if p := r.pendingNormalizedLocked(s, normalize); p != "" {
				texts = append(texts, p)
			}
		}
	} else if p := r.pendingNormalizedLocked(stream, normalize); p != "" {
		texts = append(texts, p)
	}
	return texts
}

func (r *Ring) pendingNormalizedLocked(stream Stream, normalize func(string) string) string {
	raw := r.pending(stream).String()
	if raw == "" {
		return ""
	}
	if normalize == nil {
		return raw
	}
	return normalize(raw)
}

func (r *Ring) hasContentLocked(stream Stream) bool {
	if r.pending(stream).Len() > 0 {
		return true
	}
	for _, ln := range r.lines {
		if ln.Stream == stream {
			return true
		}
	}
	return false
}

// PreferStderr reports whether both stdout and stderr have content, so a
// DetailTail should show stderr.
func (r *Ring) PreferStderr() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hasContentLocked(StreamStderr) && r.hasContentLocked(StreamStdout)
}

// HasStream reports whether stream has completed or pending content.
func (r *Ring) HasStream(stream Stream) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hasContentLocked(stream)
}

// JoinLines joins retained lines, prepending TruncationMarker when truncated.
func JoinLines(lines []string, truncated bool) string {
	if len(lines) == 0 {
		return ""
	}
	if truncated {
		return TruncationMarker + "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}
