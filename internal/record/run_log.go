package record

import (
	"slices"
	"time"
)

// DebugRecord is one structured diagnostic journal entry (§21.3).
type DebugRecord struct {
	Time    time.Time
	Level   string
	Message string
	Fields  []Field
}

// AppendLine records one durable history line (a printed message, a debug
// history line, or the required misuse line) and returns how many lines the
// run now holds.
func (r *Run) AppendLine(line string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	return len(r.lines)
}

// LineCount is how many history lines the run holds.
func (r *Run) LineCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lines)
}

// Lines is a copy of every history line, nil when there are none.
func (r *Run) Lines() []string {
	return r.LinesFrom(0)
}

// LinesFrom is a copy of the history lines from index start on, nil when
// there are none.
func (r *Run) LinesFrom(start int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if start >= len(r.lines) {
		return nil
	}
	return append([]string(nil), r.lines[start:]...)
}

// AppendMessage records one logical user-facing message.
func (r *Run) AppendMessage(m MessageSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, m)
}

// Messages is a copy of every recorded message, nil when there are none.
func (r *Run) Messages() []MessageSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]MessageSnapshot(nil), r.messages...)
}

// AppendDebugRecord records one diagnostic journal entry.
func (r *Run) AppendDebugRecord(rec DebugRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debug = append(r.debug, rec)
}

// DebugRecordCount is how many diagnostic entries the run holds.
func (r *Run) DebugRecordCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.debug)
}

// DebugRecordTail is a copy of the newest count diagnostic entries, oldest
// first: a pane shows a window, so a frame never copies the whole journal.
func (r *Run) DebugRecordTail(count int) []DebugRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	if count <= 0 || len(r.debug) == 0 {
		return nil
	}
	return slices.Clone(r.debug[max(0, len(r.debug)-count):])
}
