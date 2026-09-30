package engine

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

const quietFrameSuffix = "· quiet"

func quietLiveOutput(t *testing.T) (*Output, *countingLiveSurface, *manualClock) {
	t.Helper()
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	return out, screen, clock
}

func liveFrameAfter(out *Output, screen *countingLiveSurface, clock *manualClock, advance time.Duration) string {
	clock.Advance(advance)
	out.mu.Lock()
	out.renderLiveLocked(true)
	out.mu.Unlock()
	return screen.latestText()
}

func TestLiveQuiet_WriterRowGoesQuietThenNewLineClearsIt(t *testing.T) {
	out, screen, clock := quietLiveOutput(t)
	task := out.Task("controller")
	writeNumberedLines(t, task, 1)

	if frame := liveFrameAfter(out, screen, clock, 59*time.Second); strings.Contains(frame, quietFrameSuffix) {
		t.Fatalf("frame at 59s is quiet:\n%s", frame)
	}
	if frame := liveFrameAfter(out, screen, clock, 2*time.Second); !strings.Contains(frame, quietFrameSuffix+" 1m") {
		t.Fatalf("frame at 61s lacks %q:\n%s", quietFrameSuffix+" 1m", frame)
	}
	if frame := liveFrameAfter(out, screen, clock, 5*time.Minute); !strings.Contains(frame, quietFrameSuffix+" 6m") {
		t.Fatalf("frame at 6m lacks %q:\n%s", quietFrameSuffix+" 6m", frame)
	}
	writeNumberedLines(t, task, 1)
	if frame := liveFrameAfter(out, screen, clock, 0); strings.Contains(frame, quietFrameSuffix) {
		t.Fatalf("frame after a new line is still quiet:\n%s", frame)
	}
}

func TestLiveQuiet_TaskWithoutWriterOutputNeverGoesQuiet(t *testing.T) {
	out, screen, clock := quietLiveOutput(t)
	out.Task("controller").Doing("starting")

	if frame := liveFrameAfter(out, screen, clock, time.Hour); strings.Contains(frame, quietFrameSuffix) {
		t.Fatalf("Writer-less task is quiet:\n%s", frame)
	}
}

func TestLiveQuiet_SnapshotStampsLastLineFromTheInjectedClock(t *testing.T) {
	out, _, clock := quietLiveOutput(t)
	task := out.Task("controller")
	clock.Advance(3 * time.Minute)
	writeNumberedLines(t, task, 1)

	if got, want := core.LiveTailOf(task.Snapshot()).LastLineAt, clock.Now(); !got.Equal(want) {
		t.Fatalf("LastLineAt = %v, want %v", got, want)
	}
}

func TestPlainQuiet_NeverEmitsQuietText(t *testing.T) {
	var buf bytes.Buffer
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", to(&buf), withClock(clock), withNoColor())
	task := out.Task("controller")
	writeNumberedLines(t, task, 1)
	clock.Advance(6 * time.Minute)
	_ = out.Close()

	if strings.Contains(buf.String(), "quiet") || strings.Contains(task.Capture().Text(), "quiet") {
		t.Fatalf("plain output mentions quiet:\n%s", buf.String())
	}
}
