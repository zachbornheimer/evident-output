package engine

import (
	"bytes"
	"io"
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Writer returns a line-buffered io.Writer for narrating a talkative child
// process: each complete line (CR or LF terminated, trimmed, non-empty)
// becomes the task's live doing-text (see Doing), and every byte is also
// retained in the task's Capture ring (get-or-create, shared with
// Task.Capture) so DetailTail has proof after Fail. Lines pass through the
// same sanitize layer as Task.Doing, so hostile escape sequences never reach
// the display. Off a TTY, these mirrored lines update the live status only —
// they never force their own durable row the way an explicit
// TaskHandle.Doing call does, since the Capture ring (and its failure-path
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
	return &phaseWriter{task: t, evidence: t.Capture()}
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
	evidence *evidence

	mu  sync.Mutex
	buf []byte
}

func (w *phaseWriter) Write(p []byte) (int, error) {
	if w.evidence != nil {
		_, _ = w.evidence.Write(p)
	}
	retained := w.evidence.retainedLineCount()

	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		completed, width := lineEnding(w.buf[i:])
		w.buf = w.buf[i+width:]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case completed:
			w.task.appendLiveTail(trimmed, retained)
		default:
			w.task.setLiveOnlyPhase(trimmed)
		}
	}
	if leftover := strings.TrimSpace(string(w.buf)); leftover != "" {
		w.task.setLiveOnlyPhase(leftover)
		if len(w.buf) > phaseWriterMaxPendingBytes {
			w.buf = w.buf[:0]
		}
	}
	return len(p), nil
}

// lineEnding reads the delimiter at the head of rest: LF or CRLF completes a
// line; a lone CR is a progress frame redrawn in place, so it updates the
// phase without entering the live tail. width is the delimiter's byte count.
func lineEnding(rest []byte) (completed bool, width int) {
	if rest[0] == '\n' {
		return true, 1
	}
	if len(rest) > 1 && rest[1] == '\n' {
		return true, 2
	}
	return false, 1
}

// retainedLineCount is how many lines c's ring holds right now. It takes
// only c's own lock, so callers read it before taking the Output's.
func (c *evidence) retainedLineCount() int {
	root := c.root()
	if root == nil {
		return 0
	}
	root.mu.Lock()
	defer root.mu.Unlock()
	return len(root.lines)
}

// appendLiveTail records one completed Writer line: it enters the live tail
// and becomes the phase, exactly as a mirrored line always has.
func (t *TaskHandle) appendLiveTail(line string, retained int) {
	t.annotate(func(st *taskState) {
		line := txt.Text(line)
		st.tail.push(line, retained)
		t.out.setLiveOnlyPhaseLocked(st, line)
	})
}

// liveTailLines is how many completed Writer lines a live frame shows under
// a Running row. Fixed: the tail never grows with uptime; the Capture ring
// keeps the full record and the footer counts what the tail left out.
const liveTailLines = 6

// liveTail is a Task's most recent completed Writer lines, oldest first, at
// most liveTailLines of them.
type liveTail struct {
	lines []string
	// retained is the Capture ring's line count as of the latest push.
	retained int
}

// push appends line, evicting the oldest once full. Every completed line
// counts, even one identical to the newest: it is new output, not a redraw.
func (t *liveTail) push(line string, retained int) {
	t.retained = retained
	if len(t.lines) == liveTailLines {
		copy(t.lines, t.lines[1:])
		t.lines[liveTailLines-1] = line
		return
	}
	if t.lines == nil {
		t.lines = make([]string, 0, liveTailLines)
	}
	t.lines = append(t.lines, line)
}

// view is the tail as a snapshot sees it, sharing t's lines (see
// taskState.view).
func (t *liveTail) view() core.LiveTail {
	return core.LiveTail{Lines: t.lines, Older: max(t.retained-len(t.lines), 0)}
}

var _ io.Writer = (*phaseWriter)(nil)
