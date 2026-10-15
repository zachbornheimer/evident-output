package live

import (
	"sync"
	"testing"
	"time"
)

// paintRecorder is a LiveSurface that records what reached it. A nonzero
// delay makes each WriteLive cost that much wall time.
type paintRecorder struct {
	mu     sync.Mutex
	writes []string
	clears int
	delay  time.Duration
	starts []time.Time
}

func (s *paintRecorder) ID() string          { return "paint-recorder" }
func (s *paintRecorder) Columns() int        { return 80 }
func (s *paintRecorder) Rows() int           { return 24 }
func (s *paintRecorder) IsInteractive() bool { return true }
func (s *paintRecorder) WriteDurable(string) {}
func (s *paintRecorder) WriteFinal(string)   {}
func (s *paintRecorder) WriteLive(text string) {
	s.mu.Lock()
	s.starts = append(s.starts, time.Now())
	s.writes = append(s.writes, text)
	s.mu.Unlock()
	time.Sleep(s.delay)
}

func (s *paintRecorder) ClearLive() {
	s.mu.Lock()
	s.clears++
	s.mu.Unlock()
}

func (s *paintRecorder) written() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.writes...)
}

func (s *paintRecorder) writeStarts() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.starts...)
}

func (s *paintRecorder) clearCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clears
}

func frameAt(s Surface, seq uint64) frame { return frame{surface: s, seq: seq} }

func engineOn(s Surface) *Engine { return &Engine{cfg: Config{Surface: s}} }

func TestPaintDropsAFrameOlderThanOnePainted(t *testing.T) {
	s := &paintRecorder{}
	l := engineOn(s)
	if !l.paint(frameAt(s, 2), "new") {
		t.Fatal("frame 2 was not painted")
	}
	if l.paint(frameAt(s, 1), "old") {
		t.Fatal("frame 1 painted after frame 2")
	}
	if got := s.written(); len(got) != 1 || got[0] != "new" {
		t.Fatalf("writes = %q, want only the newer frame", got)
	}
}

func TestPaintDropsFramesAtOrBelowTheClearFloor(t *testing.T) {
	s := &paintRecorder{}
	l := engineOn(s)
	l.seq = 5
	l.paint(frameAt(s, 5), "before")
	l.Clear()
	for _, seq := range []uint64{4, 5} {
		if l.paint(frameAt(s, seq), "stale") {
			t.Fatalf("frame %d painted at or below the floor", seq)
		}
	}
	if !l.paint(frameAt(s, 6), "after") {
		t.Fatal("frame 6, past the floor, was dropped")
	}
}

func TestPaintOfIdenticalTextStillAdvancesThePaintedSequence(t *testing.T) {
	s := &paintRecorder{}
	l := engineOn(s)
	l.paint(frameAt(s, 1), "same")
	if l.paint(frameAt(s, 3), "same") {
		t.Fatal("identical text was written again")
	}
	if l.paint(frameAt(s, 2), "older") {
		t.Fatal("frame 2 painted after frame 3 was seen")
	}
	if got := len(s.written()); got != 1 {
		t.Fatalf("%d writes, want 1", got)
	}
}

func TestClearReportsWhetherAnythingWasOnScreen(t *testing.T) {
	s := &paintRecorder{}
	l := engineOn(s)
	if l.Clear() {
		t.Fatal("clear of an empty screen reported a region")
	}
	l.paint(frameAt(s, 1), "frame")
	if !l.Clear() {
		t.Fatal("clear did not report the painted region")
	}
	if l.Clear() {
		t.Fatal("second clear reported a region")
	}
	if got := s.clearCount(); got != 1 {
		t.Fatalf("ClearLive called %d times, want 1", got)
	}
}

// TestFrameSnapshottedBeforeAClearIsNeverPainted is the Close/finish/durable
