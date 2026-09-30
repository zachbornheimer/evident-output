package live

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

var quietT0 = time.Date(2026, 9, 29, 16, 0, 0, 0, time.UTC)

func quietTask(lastLine time.Time) core.TaskSnapshot {
	tail := core.LiveTail{Lines: []string{"heartbeat workers=3"}, Evidence: 1, LastLineAt: lastLine}
	return core.WithLiveTail(core.TaskSnapshot{Name: "controller", State: core.Running}, tail)
}

func ownerRowAt(t core.TaskSnapshot, st liveStyle, elapsed time.Duration) string {
	var b strings.Builder
	st.now = quietT0.Add(elapsed)
	writeLiveTaskLine(&b, t, 0, 0, st)
	owner, _, _ := strings.Cut(b.String(), "\n")
	return owner
}

func TestLiveQuiet_SuffixAppearsPastThreshold(t *testing.T) {
	t.Parallel()
	task := quietTask(quietT0)
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{59 * time.Second, "⠋ controller"},
		{61 * time.Second, "⠋ controller  · quiet 1m"},
		{6 * time.Minute, "⠋ controller  · quiet 6m"},
		{2*time.Hour + 10*time.Minute, "⠋ controller  · quiet 2h10m"},
	}
	for _, tc := range cases {
		if got := ownerRowAt(task, testLiveStyle, tc.elapsed); got != tc.want {
			t.Errorf("at %s owner row = %q, want %q", tc.elapsed, got, tc.want)
		}
	}
}

func TestLiveQuiet_NewLineClearsSuffix(t *testing.T) {
	t.Parallel()
	task := quietTask(quietT0.Add(6 * time.Minute))
	if got, want := ownerRowAt(task, testLiveStyle, 6*time.Minute+time.Second), "⠋ controller"; got != want {
		t.Fatalf("owner row = %q, want %q", got, want)
	}
}

func TestLiveQuiet_TaskWithoutWriterOutputIsNeverQuiet(t *testing.T) {
	t.Parallel()
	task := core.TaskSnapshot{Name: "controller", State: core.Running}
	if got := ownerRowAt(task, testLiveStyle, time.Hour); strings.Contains(got, "quiet") {
		t.Fatalf("owner row = %q, want no quiet suffix", got)
	}
}

func TestLiveQuiet_SettledTaskIsNeverQuiet(t *testing.T) {
	t.Parallel()
	task := quietTask(quietT0)
	task.State = core.Done
	if got := ownerRowAt(task, testLiveStyle, time.Hour); strings.Contains(got, "quiet") {
		t.Fatalf("owner row = %q, want no quiet suffix", got)
	}
}

func TestLiveQuiet_SuffixIsWarnColoredWhenColorIsOn(t *testing.T) {
	t.Parallel()
	st := testLiveStyle
	st.Color = true
	got := ownerRowAt(quietTask(quietT0), st, 6*time.Minute)
	if want := txt.Style("· quiet 6m", txt.SGRYellow, true); !strings.Contains(got, want) {
		t.Fatalf("owner row = %q, want it to contain %q", got, want)
	}
}

func TestLiveQuiet_NarrowWidthKeepsSpinnerAndName(t *testing.T) {
	t.Parallel()
	st := testLiveStyle
	st.width = 16
	got := ownerRowAt(quietTask(quietT0), st, 6*time.Minute)
	if cells := txt.VisibleCells(got); cells > st.width {
		t.Fatalf("owner row %q is %d cells, want <= %d", got, cells, st.width)
	}
	if !strings.HasPrefix(got, "⠋ controller") {
		t.Fatalf("owner row = %q, want spinner and name intact", got)
	}
}
