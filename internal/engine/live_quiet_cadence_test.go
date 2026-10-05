package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

type quietCadenceSurface struct {
	mu     sync.Mutex
	frames []quietCadenceFrame
}

type quietCadenceFrame struct {
	at   time.Time
	text string
}

func (s *quietCadenceSurface) ID() string          { return "quiet-cadence" }
func (s *quietCadenceSurface) Columns() int        { return 80 }
func (s *quietCadenceSurface) Rows() int           { return 24 }
func (s *quietCadenceSurface) IsInteractive() bool { return true }
func (s *quietCadenceSurface) ClearLive()          {}
func (s *quietCadenceSurface) WriteDurable(string) {}
func (s *quietCadenceSurface) WriteFinal(string)   {}
func (s *quietCadenceSurface) WriteLive(text string) {
	s.mu.Lock()
	s.frames = append(s.frames, quietCadenceFrame{at: time.Now(), text: text})
	s.mu.Unlock()
}

func (s *quietCadenceSurface) snapshot() []quietCadenceFrame {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames := make([]quietCadenceFrame, len(s.frames))
	copy(frames, s.frames)
	return frames
}

const (
	quietCadenceWindow   = 1200 * time.Millisecond
	quietCadenceMaxGap   = 100 * time.Millisecond
	quietCadenceMinFrame = 10
)

// TestLive_QuietRunningTaskKeepsChangingFrames proves the interactive live
// projection remains meaningfully alive without new application output. Every
// frame written during the quiet interval is a changed frame by construction,
// because renderLiveLocked suppresses identical text before WriteLive. No
// stale interval may open at either edge of the window either.
func TestLive_QuietRunningTaskKeepsChangingFrames(t *testing.T) {
	surface := &quietCadenceSurface{}
	out := newOutput("job", withTerminal(surface), visibilityDelay(0), withNoColor())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("quiet")
	task.Doing("working")
	activated := time.Now()
	time.Sleep(quietCadenceWindow)
	end := time.Now()

	if state := task.Snapshot().State; state != Running {
		t.Fatalf("task state at end of quiet window = %v, want Running", state)
	}

	var frames []quietCadenceFrame
	for _, f := range surface.snapshot() {
		if !f.at.Before(activated) {
			frames = append(frames, f)
		}
	}
	if len(frames) < quietCadenceMinFrame {
		t.Fatalf("quiet Running task produced %d changed live frames, want at least %d", len(frames), quietCadenceMinFrame)
	}
	for i, f := range frames {
		if !strings.Contains(f.text, "quiet") {
			t.Fatalf("frame %d is not the rendered task row: %q", i, f.text)
		}
	}
	if lead := frames[0].at.Sub(activated); lead >= quietCadenceMaxGap {
		t.Fatalf("first frame arrived %s after activation, want <%s", lead, quietCadenceMaxGap)
	}
	for i := 1; i < len(frames); i++ {
		if frames[i].text == frames[i-1].text {
			t.Fatalf("frame %d repeated frame text despite WriteLive deduplication", i)
		}
		if gap := frames[i].at.Sub(frames[i-1].at); gap >= quietCadenceMaxGap {
			t.Fatalf("quiet live frame gap %s at frame %d, want <%s", gap, i, quietCadenceMaxGap)
		}
	}
	if tail := end.Sub(frames[len(frames)-1].at); tail >= quietCadenceMaxGap {
		t.Fatalf("last frame is %s stale at end of window, want <%s", tail, quietCadenceMaxGap)
	}
}

func TestUntilNextSpinnerSlot(t *testing.T) {
	period := int64(txt.SpinnerPeriod)
	base := time.Unix(0, 1000*period)
	cases := map[string]time.Time{
		"on boundary": base,
		"just after":  base.Add(time.Nanosecond),
		"just before": base.Add(txt.SpinnerPeriod - time.Nanosecond),
		"mid slot":    base.Add(txt.SpinnerPeriod / 2),
	}
	for name, now := range cases {
		t.Run(name, func(t *testing.T) {
			wait := untilNextSpinnerSlot(now)
			if wait <= 0 || wait > txt.SpinnerPeriod+spinnerSlotSettle {
				t.Fatalf("wait %s outside (0, %s]", wait, txt.SpinnerPeriod+spinnerSlotSettle)
			}
			current := now.UnixNano() / period
			if got := now.Add(wait).UnixNano() / period; got != current+1 {
				t.Fatalf("wake lands in slot %d, want %d", got, current+1)
			}
		})
	}
}
