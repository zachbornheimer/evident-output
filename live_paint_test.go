package evo

import (
	"sync"
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
