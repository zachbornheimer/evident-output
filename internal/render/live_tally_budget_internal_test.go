package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// shortTerminalRows is a live height the header, the folded tally and one
// running child fill exactly once the tally drops its cause line.
const shortTerminalRows = 5

// causedItems is a Group of running children plus Skipped item children
// under two reasons whose records each carry a cause, so the tally would
// also print a └─ evidence line.
func causedItems(running int) core.TasksSnapshot {
	var tasks []core.TaskSnapshot
	for i := range running {
		tasks = append(tasks, core.TaskSnapshot{Name: fmt.Sprintf("package-%d", i), State: core.Running, Phase: "downloading"})
	}
	for i := range 2 {
		name := fmt.Sprintf("item-%d", i)
		pinned := []core.TaxonomyRecord{{Reason: "pinned", Name: name, Causes: []string{"lockfile pins " + name}}}
		vendored := []core.TaxonomyRecord{{Reason: "vendored", Name: name, Causes: []string{"vendor/ holds " + name}}}
		tasks = append(tasks,
			core.TaskSnapshot{Name: "pinned-" + name, State: core.Skipped, Skipped: pinned},
			core.TaskSnapshot{Name: "vendored-" + name, State: core.Skipped, Skipped: vendored})
	}
	return core.TasksSnapshot{Name: "packages", State: core.Running, Tasks: tasks}
}

// TestLiveTallies_DropCausesBeforeOverrunningTheHeight: when the tallies'
// cause lines would push a live Group past the terminal height, the frame
// keeps each tally's headline and drops its causes (the durable render
// still carries them).
func TestLiveTallies_DropCausesBeforeOverrunningTheHeight(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, causedItems(3), shortTerminalRows, testLiveStyle)
	frame := b.String()
	if rows := strings.Count(frame, "\n"); rows > shortTerminalRows {
		t.Fatalf("live frame is %d rows, over the %d-row height:\n%s", rows, shortTerminalRows, frame)
	}
	for _, want := range []string{"- skipped 4 (2 pinned, 2 vendored)", "package-0"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("live frame lacks %q:\n%s", want, frame)
		}
	}
}

// TestLiveTallies_KeepCausesWhenTheyFit: with room to spare the live
// tallies keep their cause lines.
func TestLiveTallies_KeepCausesWhenTheyFit(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, causedItems(1), 20, testLiveStyle)
	if frame := b.String(); !strings.Contains(frame, "lockfile pins item-0 (+3 more)") {
		t.Fatalf("live frame dropped causes it had room for:\n%s", frame)
	}
}
