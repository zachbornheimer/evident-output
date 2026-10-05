package engine

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// tailTestLines is far more Writer output than the live tail can show.
const tailTestLines = 500

func writeNumberedLines(t *testing.T, task *TaskHandle, n int) {
	t.Helper()
	w := task.Writer()
	for i := 1; i <= n; i++ {
		if _, err := fmt.Fprintf(w, "line %03d\n", i); err != nil {
			t.Fatal(err)
		}
	}
}

func numberedLines(from, to int) []string {
	var lines []string
	for i := from; i <= to; i++ {
		lines = append(lines, fmt.Sprintf("line %03d", i))
	}
	return lines
}

// TestWriter_LiveTailStaysBoundedWhileCaptureKeepsTheStream proves the live
// tail holds only the newest liveTailLines lines however long the child
// runs, while the Capture ring still retains the stream behind it.
func TestWriter_LiveTailStaysBoundedWhileCaptureKeepsTheStream(t *testing.T) {
	out := newOutput("job", to(&bytes.Buffer{}), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("build")

	writeNumberedLines(t, task, tailTestLines)

	tail := core.LiveTailOf(task.Snapshot())
	if want := numberedLines(tailTestLines-liveTailLines+1, tailTestLines); !slices.Equal(tail.Lines, want) {
		t.Fatalf("tail = %q, want the newest %d lines %q", tail.Lines, liveTailLines, want)
	}
	retained := strings.Split(task.Capture().Text(), "\n")
	if want := numberedLines(tailTestLines-defaultCaptureLines+1, tailTestLines); !slices.Equal(retained[len(retained)-len(want):], want) {
		t.Fatalf("Capture lost the stream: retained %d lines ending %q", len(retained), retained[len(retained)-1])
	}
	if want := defaultCaptureLines - liveTailLines; tail.Older != want {
		t.Fatalf("tail.Older = %d, want %d retained lines above the tail", tail.Older, want)
	}
	out.mu.Lock()
	capacity := cap(out.taskByRef[task.id].tail.lines)
	out.mu.Unlock()
	if capacity != liveTailLines {
		t.Fatalf("tail backing capacity = %d after %d lines, want fixed %d", capacity, tailTestLines, liveTailLines)
	}
}

// TestWriter_ShortStreamHasNoOlderLines proves the footer count is zero
// while every retained line is already visible.
func TestWriter_ShortStreamHasNoOlderLines(t *testing.T) {
	out := newOutput("job", to(&bytes.Buffer{}), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("build")

	writeNumberedLines(t, task, 3)

	tail := core.LiveTailOf(task.Snapshot())
	if !slices.Equal(tail.Lines, numberedLines(1, 3)) || tail.Older != 0 {
		t.Fatalf("tail = %+v, want 3 lines and no older count", tail)
	}
}

// TestWriter_DuplicateLinesEachEnterTheTail proves a completed line equal to
// the newest tail line is still a new line of output: consecutive duplicates
// occupy separate tail entries, and the tail stays bounded.
func TestWriter_DuplicateLinesEachEnterTheTail(t *testing.T) {
	out := newOutput("job", to(&bytes.Buffer{}), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("poll")
	w := task.Writer()

	for range liveTailLines * 3 {
		if _, err := fmt.Fprintln(w, "waiting"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fmt.Fprint(w, "ready\nready\n"); err != nil {
		t.Fatal(err)
	}

	want := slices.Repeat([]string{"waiting"}, liveTailLines-2)
	want = append(want, "ready", "ready")
	tail := core.LiveTailOf(task.Snapshot())
	if !slices.Equal(tail.Lines, want) {
		t.Fatalf("tail = %q, want %q", tail.Lines, want)
	}
	if wantOlder := liveTailLines*3 + 2 - liveTailLines; tail.Older != wantOlder {
		t.Fatalf("tail.Older = %d, want %d", tail.Older, wantOlder)
	}
}

// TestWriter_CRFramesStayOutOfTheTail proves a lone-CR progress frame only
// updates the phase: redrawn-in-place frames are not lines of output.
func TestWriter_CRFramesStayOutOfTheTail(t *testing.T) {
	out := newOutput("job", to(&bytes.Buffer{}), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("download")

	if _, err := task.Writer().Write([]byte("fetching\r\n10%\r50%\r")); err != nil {
		t.Fatal(err)
	}

	snap := task.Snapshot()
	if tail := core.LiveTailOf(snap); !slices.Equal(tail.Lines, []string{"fetching"}) {
		t.Fatalf("tail = %q, want only the CRLF-completed line", tail.Lines)
	}
	if snap.Phase != "50%" {
		t.Fatalf("phase = %q, want the latest CR frame", snap.Phase)
	}
}

// TestWriter_LiveFrameShowsOwnerTailAndFooter proves a TTY frame keeps the
// Writer task's own row and draws the bounded tail and footer beneath it.
func TestWriter_LiveFrameShowsOwnerTailAndFooter(t *testing.T) {
	screen := &countingLiveSurface{}
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	out := newOutput("job", withTerminal(screen), visibilityDelay(0), withClock(clock), withNoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("build")

	writeNumberedLines(t, task, 10)

	frame := strings.Split(screen.latestText(), "\n")
	if len(frame) != 1+liveTailLines+1 {
		t.Fatalf("frame has %d rows, want owner + %d tail + footer:\n%s", len(frame), liveTailLines, strings.Join(frame, "\n"))
	}
	if !strings.Contains(frame[0], "build") || strings.Contains(frame[0], "line") {
		t.Fatalf("owner row = %q, want the task name without the tail's newest line", frame[0])
	}
	for i, want := range numberedLines(5, 10) {
		if got := strings.TrimSpace(frame[1+i]); got != want {
			t.Fatalf("tail row %d = %q, want %q", i, got, want)
		}
	}
	if !strings.Contains(frame[len(frame)-1], "4 earlier lines retained") {
		t.Fatalf("footer = %q, want the 4 retained lines above the tail", frame[len(frame)-1])
	}
}

// TestWriter_PlainOutputDoesNotRepeatTailRows proves off-TTY Writer lines
// stay out of the durable stream: the tail is a live-frame projection only.
func TestWriter_PlainOutputDoesNotRepeatTailRows(t *testing.T) {
	var primary bytes.Buffer
	out := newOutput("job", to(&primary), withNoColor())
	task := out.Task("build")

	writeNumberedLines(t, task, 20)
	task.Summary("built")
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(primary.String(), "line 0") || strings.Contains(primary.String(), "earlier lines") {
		t.Fatalf("plain output repeated Writer tail rows:\n%s", primary.String())
	}
}
