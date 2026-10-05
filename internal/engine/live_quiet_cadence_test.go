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

	// quietCadenceAttempts bounds the retries of an attempt that the host
	// starved too badly to prove the bound either way.
	quietCadenceAttempts = 6

	// hostStallProbePeriod is how often the witness goroutine wakes, and
	// hostStallThreshold the lateness that proves the host withheld the CPU
	// from this process: half of what the bound leaves over one spinner period.
	hostStallProbePeriod = 2 * time.Millisecond
	hostStallThreshold   = 20 * time.Millisecond
)

// hostStallWitness records, from an independent goroutine, intervals in which
// this process was not scheduled. A stale interval that overlaps one cannot be
// told from a product defect by the frame times alone, so the attempt is
// unprovable (retried, never passed), as scripts/verify-quiet-pty-stream.py
// treats a reader that was itself descheduled.
type hostStallWitness struct {
	mu     sync.Mutex
	stalls []quietCadenceInterval
	stop   chan struct{}
	done   chan struct{}
}

type quietCadenceInterval struct{ from, to time.Time }

func (iv quietCadenceInterval) length() time.Duration { return iv.to.Sub(iv.from) }

func (iv quietCadenceInterval) overlaps(other quietCadenceInterval) bool {
	return iv.from.Before(other.to) && other.from.Before(iv.to)
}

func startHostStallWitness() *hostStallWitness {
	w := &hostStallWitness{stop: make(chan struct{}), done: make(chan struct{})}
	go w.run()
	return w
}

func (w *hostStallWitness) run() {
	defer close(w.done)
	last := time.Now()
	for {
		select {
		case <-w.stop:
			return
		case <-time.After(hostStallProbePeriod):
		}
		now := time.Now()
		if now.Sub(last)-hostStallProbePeriod >= hostStallThreshold {
			w.mu.Lock()
			w.stalls = append(w.stalls, quietCadenceInterval{from: last, to: now})
			w.mu.Unlock()
		}
		last = now
	}
}

func (w *hostStallWitness) close() {
	close(w.stop)
	<-w.done
}

func (w *hostStallWitness) overlapping(iv quietCadenceInterval) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, s := range w.stalls {
		if s.overlaps(iv) {
			return true
		}
	}
	return false
}

// staleIntervals are the spans of the window in which no changed frame was on
// screen for quietCadenceMaxGap or more: the lead from activation, every gap
// between frames, and the tail to the window's end.
func staleIntervals(frames []quietCadenceFrame, activated, end time.Time) []quietCadenceInterval {
	var stale []quietCadenceInterval
	add := func(from, to time.Time) {
		if iv := (quietCadenceInterval{from: from, to: to}); iv.length() >= quietCadenceMaxGap {
			stale = append(stale, iv)
		}
	}
	add(activated, frames[0].at)
	for i := 1; i < len(frames); i++ {
		add(frames[i-1].at, frames[i].at)
	}
	add(frames[len(frames)-1].at, end)
	return stale
}

// observeQuietWindow runs one quiet Running Task for quietCadenceWindow and
// returns the changed frames written during it, after the content checks.
func observeQuietWindow(t *testing.T) (frames []quietCadenceFrame, activated, end time.Time) {
	t.Helper()
	surface := &quietCadenceSurface{}
	out := newOutput("job", withTerminal(surface), visibilityDelay(0), withNoColor())
	defer func() { _ = out.Close() }()

	task := out.Task("quiet")
	task.Doing("working")
	activated = time.Now()
	time.Sleep(quietCadenceWindow)
	end = time.Now()

	if state := task.Snapshot().State; state != Running {
		t.Fatalf("task state at end of quiet window = %v, want Running", state)
	}
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
		if i > 0 && f.text == frames[i-1].text {
			t.Fatalf("frame %d repeated frame text despite WriteLive deduplication", i)
		}
	}
	return frames, activated, end
}

// TestLive_QuietRunningTaskKeepsChangingFrames proves the interactive live
// projection remains meaningfully alive without new application output. Every
// frame written during the quiet interval is a changed frame by construction,
// because renderLiveLocked suppresses identical text before WriteLive. No
// stale interval of quietCadenceMaxGap or more may open anywhere in the window,
// at either edge included. A stale interval that coincides with a proven host
// stall (hostStallWitness) cannot prove the bound, so that attempt is retried
// and never passes; a stale interval with the process scheduled fails at once.
func TestLive_QuietRunningTaskKeepsChangingFrames(t *testing.T) {
	witness := startHostStallWitness()
	defer witness.close()

	for attempt := 1; attempt <= quietCadenceAttempts; attempt++ {
		frames, activated, end := observeQuietWindow(t)
		stale := staleIntervals(frames, activated, end)
		if len(stale) == 0 {
			return
		}
		for _, iv := range stale {
			if !witness.overlapping(iv) {
				t.Fatalf("quiet live frames stale %s (%s to %s) with the process scheduled, want <%s",
					iv.length(), iv.from.Format("15:04:05.000"), iv.to.Format("15:04:05.000"), quietCadenceMaxGap)
			}
		}
		t.Logf("attempt %d unprovable: host stalled the process across %d stale interval(s)", attempt, len(stale))
	}
	t.Fatalf("unprovable under current load: %d attempts each overlapped a host stall", quietCadenceAttempts)
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

func TestStaleIntervalsFindsEveryEdgeAndGap(t *testing.T) {
	base := time.Unix(1000, 0)
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	frames := []quietCadenceFrame{{at: at(150)}, {at: at(200)}, {at: at(330)}, {at: at(380)}}

	got := staleIntervals(frames, at(0), at(500))

	want := []quietCadenceInterval{{at(0), at(150)}, {at(200), at(330)}, {at(380), at(500)}}
	if len(got) != len(want) {
		t.Fatalf("stale intervals = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stale interval %d = %v, want %v", i, got[i], want[i])
		}
	}
	if stale := staleIntervals(frames[1:2], at(150), at(240)); len(stale) != 0 {
		t.Fatalf("a window with no gap of %s reported stale %v", quietCadenceMaxGap, stale)
	}
}

func TestHostStallWitnessOnlyExcusesOverlappingIntervals(t *testing.T) {
	base := time.Unix(1000, 0)
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	w := &hostStallWitness{stalls: []quietCadenceInterval{{at(100), at(160)}}}

	if !w.overlapping(quietCadenceInterval{at(50), at(150)}) {
		t.Fatal("interval spanning a host stall was not excused")
	}
	if w.overlapping(quietCadenceInterval{at(200), at(320)}) {
		t.Fatal("interval clear of every host stall was excused")
	}
}
