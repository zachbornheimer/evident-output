package evo

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// Writer returns a line-buffered io.Writer for narrating a talkative child
// process: each complete line (CR or LF terminated, trimmed, non-empty)
// becomes the task's live doing-text (see Doing), and every byte is also
// retained in the task's Evidence ring (get-or-create, shared with
// Task.Evidence) so DetailTail has proof after Fail. Lines pass through the
// same sanitize layer as Task.Doing, so hostile escape sequences never reach
// the display. Off a TTY, these mirrored lines update the live status only —
// they never force their own durable row the way an explicit
// TaskHandle.Doing call does, since the Evidence ring (and its failure-path
// DetailTail) is already the child's one durable home (release-gate round 9
// finding 4). Concurrent-safe. Named Writer, not PhaseWriter (P6/rename):
// an io.Writer sink whose lines become the live-status text, following
// logrus's Logger.Writer()/zapio.Writer precedent.
//
//	cmd.Stdout = evo.Task("push").Writer()
func (t *TaskHandle) Writer() io.Writer {
	if t == nil || t.out == nil {
		return io.Discard
	}
	return &phaseWriter{task: t, evidence: t.Evidence()}
}

// phaseWriterMaxPendingBytes bounds the pending-line buffer: a child that
// never emits a line terminator (or emits one far longer than any phase
// text should be) would otherwise grow this buffer without limit. Once the
// pending fragment reaches this size, it is flushed as a phase line on its
// own — every byte still lands in Capture regardless, so no evidence is
// lost, only the "one line, one phase update" grouping is.
const phaseWriterMaxPendingBytes = 4 * 1024 // 4 KiB

// phaseWriter splits child output into lines (both CR and LF delimit, so a
// \r-driven progress bar updates Phase every frame) and forwards the trimmed
// text to TaskHandle.Phase, while every raw byte still feeds capture. The
// pending (not-yet-terminated) fragment is capped at
// phaseWriterMaxPendingBytes so a line-less/oversized child stream cannot
// grow it without bound.
type phaseWriter struct {
	task     *TaskHandle
	evidence *Evidence

	mu  sync.Mutex
	buf []byte
}

func (w *phaseWriter) Write(p []byte) (int, error) {
	if w.evidence != nil {
		_, _ = w.evidence.Write(p)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			w.task.setLiveOnlyPhase(trimmed)
		}
	}
	if len(w.buf) > phaseWriterMaxPendingBytes {
		if trimmed := strings.TrimSpace(string(w.buf)); trimmed != "" {
			w.task.setLiveOnlyPhase(trimmed)
		}
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

var _ io.Writer = (*phaseWriter)(nil)
