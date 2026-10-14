package live

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// tailLines is how many completed Writer lines a frame shows under a
// Running row. Fixed: the tail never grows with uptime; the Capture ring
// keeps the full record and the footer reports how many lines it holds.
const tailLines = 6

// tail is a Task's most recent completed Writer lines, oldest first, at
// most tailLines of them.
type tail struct {
	lines []string
	// retained is the Capture ring's line count as of the latest push.
	retained int
	// lastLineAt is the domain-clock time of the latest push.
	lastLineAt time.Time
}

// push appends line, evicting the oldest once full. Every completed line
// counts, even one identical to the newest: it is new output, not a redraw.
func (t *tail) push(line string, retained int, at time.Time) {
	t.retained = retained
	t.lastLineAt = at
	if len(t.lines) == tailLines {
		copy(t.lines, t.lines[1:])
		t.lines[tailLines-1] = line
		return
	}
	if t.lines == nil {
		t.lines = make([]string, 0, tailLines)
	}
	t.lines = append(t.lines, line)
}

// view is the tail as a snapshot carries it, sharing t's lines.
func (t *tail) view() record.LiveTail {
	return record.LiveTail{Lines: t.lines, Evidence: t.retained, LastLineAt: t.lastLineAt}
}
