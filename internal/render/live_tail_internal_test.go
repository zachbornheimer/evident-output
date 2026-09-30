package render

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func tailedTask(state core.EntityState, tail core.LiveTail) core.TaskSnapshot {
	t := core.TaskSnapshot{Name: "build", State: state, Phase: tail.Lines[len(tail.Lines)-1]}
	return core.WithLiveTail(t, tail)
}

func TestWriteLiveTaskLine_RunningTailRendersUnderOwnerWithFooter(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	tail := core.LiveTail{Lines: []string{"a", "b", "c", "d", "e", "f"}, Evidence: 214}

	rows := writeLiveTaskLine(&b, tailedTask(core.Running, tail), 0, 0, testLiveStyle)

	want := "⠋ build\n   a\n   b\n   c\n   d\n   e\n   f\n   … 214 lines in evidence\n"
	if got := b.String(); got != want || rows != 8 {
		t.Fatalf("rows=%d\n%s\nwant 8 rows:\n%s", rows, got, want)
	}
}

func TestWriteLiveTaskLine_TailHoldingAllEvidenceHasNoFooter(t *testing.T) {
	t.Parallel()
	var b strings.Builder

	writeLiveTaskLine(&b, tailedTask(core.Running, core.LiveTail{Lines: []string{"only"}}), 0, 0, testLiveStyle)

	if got, want := b.String(), "⠋ build\n   only\n"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteLiveTaskLine_TailLinesFitTheFrameWidth(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	st := testLiveStyle
	st.width = 30
	long := strings.Repeat("compiling a very long path ", 20)
	tail := core.LiveTail{Lines: []string{long, long + "1", long + "2"}, Evidence: 7}

	rows := writeLiveTaskLine(&b, tailedTask(core.Running, tail), 0, 0, st)

	if rows != 5 {
		t.Fatalf("rows = %d, want owner + 3 tail + footer (no wrapping):\n%s", rows, b.String())
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(b.String(), "\n"), "\n") {
		if cells := txt.VisibleCells(line); cells > st.width {
			t.Fatalf("row %q is %d cells, want <= %d", line, cells, st.width)
		}
	}
	if owner, _, _ := strings.Cut(b.String(), "\n"); owner != "⠋ build" {
		t.Fatalf("owner row = %q, want it unstretched by the tail", owner)
	}
}

func TestWriteLiveTaskLine_SettledTaskDropsItsTail(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	tail := core.LiveTail{Lines: []string{"x"}, Evidence: 3}

	rows := writeLiveTaskLine(&b, tailedTask(core.Done, tail), 0, 0, testLiveStyle)

	if rows != 1 || strings.Contains(b.String(), "evidence") {
		t.Fatalf("settled row kept its live tail:\n%s", b.String())
	}
}

func TestWriteLiveTaskLine_DistinctPhaseStaysOnOwnerRow(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	task := core.WithLiveTail(core.TaskSnapshot{Name: "build", State: core.Running, Phase: "50%"},
		core.LiveTail{Lines: []string{"fetching"}})

	writeLiveTaskLine(&b, task, 0, 0, testLiveStyle)

	if got, want := b.String(), "⠋ build  50%\n   fetching\n"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteLiveTaskLine_FooterSingularAndFitsNarrowFrame(t *testing.T) {
	t.Parallel()
	var b strings.Builder

	writeLiveTaskLine(&b, tailedTask(core.Running, core.LiveTail{Lines: []string{"a"}, Evidence: 2}), 0, 0, testLiveStyle)
	if got, want := b.String(), "⠋ build\n   a\n   … 2 lines in evidence\n"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := evidenceLinesText(1); got != "1 line in evidence" {
		t.Fatalf("singular = %q", got)
	}

	b.Reset()
	st := testLiveStyle
	st.width = 12
	writeLiveTaskLine(&b, tailedTask(core.Running, core.LiveTail{Lines: []string{"a"}, Evidence: 214}), 0, 0, st)
	footer := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")[2]
	if cells := txt.VisibleCells(footer); cells > st.width {
		t.Fatalf("footer %q is %d cells, want <= %d", footer, cells, st.width)
	}
}
