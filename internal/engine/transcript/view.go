package transcript

import (
	"fmt"
	"strings"
)

// Text returns all retained combined lines joined by newlines.
func (t *Transcript) Text() string {
	lines, truncated := t.snapshotTexts(Combined, 0)
	return joinLines(lines, truncated)
}

// StreamText returns one stream's retained lines joined by newlines, with
// no human-facing truncation marker — the caller decides how to surface
// Truncated().
func (t *Transcript) StreamText(s Stream) string {
	lines, _ := t.snapshotTexts(s, 0)
	return strings.Join(lines, "\n")
}

// Truncated reports whether the retained ring has ever dropped a line to
// stay within its bound.
func (t *Transcript) Truncated() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.truncated
}

// Empty reports whether no completed lines and no pending fragments exist.
func (t *Transcript) Empty() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.lines) > 0 {
		return false
	}
	return t.pendingFor(Combined).Len() == 0 &&
		t.pendingFor(Stdout).Len() == 0 &&
		t.pendingFor(Stderr).Len() == 0
}

// Detail is the failure-tail text viewed from s: Combined prefers Stderr
// when Stderr has content; Stdout or Stderr views that stream. Falls back
// to Combined when the view is empty.
func (t *Transcript) Detail(s Stream) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	prefer := s
	if s == Combined {
		hasOut := t.streamHasContentLocked(Stdout)
		hasErr := t.streamHasContentLocked(Stderr)
		if hasErr && hasOut {
			prefer = Stderr
		} else if hasErr {
			prefer = Stderr
		}
	}

	texts := t.textsForStreamLocked(prefer)
	// If filter emptied, fall back to all combined lines + all pendings.
	if len(texts) == 0 {
		texts = t.textsForStreamLocked(Combined)
	}
	if len(texts) == 0 {
		return ""
	}
	var b strings.Builder
	if t.truncated {
		b.WriteString(truncationMarker)
		b.WriteByte('\n')
	}
	if len(texts) > 1 {
		fmt.Fprintf(&b, "Last %d lines:\n", len(texts))
	}
	b.WriteString(strings.Join(texts, "\n"))
	return b.String()
}

func (t *Transcript) streamHasContentLocked(s Stream) bool {
	if t.pendingFor(s).Len() > 0 {
		return true
	}
	for _, ln := range t.lines {
		if ln.Stream == s {
			return true
		}
	}
	return false
}

// textsForStreamLocked returns completed lines plus pending fragments for
// s. Combined includes every stream's completed lines and all pendings.
// Pending fragments are snapshotted (not flushed) so concurrent Write stays
// safe.
func (t *Transcript) textsForStreamLocked(s Stream) []string {
	var texts []string
	for _, ln := range t.lines {
		if s == Combined || ln.Stream == s {
			texts = append(texts, ln.Text)
		}
	}
	if s == Combined {
		for _, st := range []Stream{Combined, Stdout, Stderr} {
			if p := t.pendingNormalizedLocked(st); p != "" {
				texts = append(texts, p)
			}
		}
	} else if p := t.pendingNormalizedLocked(s); p != "" {
		texts = append(texts, p)
	}
	return texts
}

func (t *Transcript) snapshotTexts(s Stream, limit int) ([]string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	texts := t.textsForStreamLocked(s)
	if limit > 0 && len(texts) > limit {
		texts = texts[len(texts)-limit:]
	}
	return append([]string(nil), texts...), t.truncated
}

// pendingNormalizedLocked returns a sanitized/redacted view of a pending
// buffer without flushing it into the ring.
func (t *Transcript) pendingNormalizedLocked(s Stream) string {
	raw := t.pendingFor(s).String()
	if raw == "" {
		return ""
	}
	return t.normalizeLine(raw)
}

func joinLines(lines []string, truncated bool) string {
	if len(lines) == 0 {
		return ""
	}
	if truncated {
		return truncationMarker + "\n" + strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n")
}
