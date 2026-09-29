package live

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// shortTerminalRows is a live height the header, both folded tallies and
// one running child fill exactly once the tallies drop their cause lines.
const shortTerminalRows = 5

// causedItems is a Group of running children plus Kept and Skipped item
// children whose records each carry a cause, so every tally would also
// print a └─ evidence line.
func causedItems(running int) core.TasksSnapshot {
	var tasks []core.TaskSnapshot
	for i := range running {
		tasks = append(tasks, core.TaskSnapshot{Name: fmt.Sprintf("package-%d", i), State: core.Running, Phase: "downloading"})
	}
	for i := range 2 {
		name := fmt.Sprintf("item-%d", i)
		rec := []core.TaxonomyRecord{{Reason: "pinned", Name: name, Causes: []string{"lockfile pins " + name}}}
		tasks = append(tasks,
			core.TaskSnapshot{Name: "kept-" + name, State: core.Done, Kept: rec},
			core.TaskSnapshot{Name: "skipped-" + name, State: core.Skipped, Skipped: rec})
	}
	return core.TasksSnapshot{Name: "packages", State: core.Running, Tasks: tasks}
}

// settled returns col with every Running Task resolved Done, so a caller
// exercising the folded tallies (which paint only once the category itself
// has stopped classifying — contract §18) isn't accidentally testing the
// running-suppression rule instead of what it means to test. Once those
// children are terminal they are work peers of their own (IsWorkPeer), so
// col gets a Summary too: folds() only tolerates a work peer beside a
// folded tally when the Group names its own subject with one (the same
// rule a finished "12 checked" category already relies on).
func settled(col core.TasksSnapshot) core.TasksSnapshot {
	tasks := make([]core.TaskSnapshot, len(col.Tasks))
	copy(tasks, col.Tasks)
	for i := range tasks {
		if tasks[i].State == core.Running {
			tasks[i].State = core.Done
			tasks[i].Phase = ""
			tasks[i].Summary = "downloaded"
		}
	}
	col.Tasks = tasks
	col.State = core.Done
	col.Summary = "downloads complete"
	return col
}

// TestLiveTallies_DropCausesBeforeOverrunningTheHeight: when the tallies'
// cause lines would push a live Group past the terminal height, the frame
// keeps each tally's headline and drops its causes (the durable render
// still carries them). settled's filler children outrank neither
// Disposition (they render, if room allows, as ordinary settled rows —
// contract §18's running-suppression rule does not gate them once they,
// and the whole category, have stopped running), so this only pins the
// tallies' own row.
func TestLiveTallies_DropCausesBeforeOverrunningTheHeight(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, settled(causedItems(3)), shortTerminalRows, testLiveStyle)
	frame := b.String()
	if rows := strings.Count(frame, "\n"); rows > shortTerminalRows {
		t.Fatalf("live frame is %d rows, over the %d-row height:\n%s", rows, shortTerminalRows, frame)
	}
	for _, want := range []string{"- skipped 2 (pinned)", "! kept 2 (pinned)"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("live frame lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "lockfile pins") {
		t.Fatalf("live frame kept a cause line it had no room for:\n%s", frame)
	}
}

// TestLiveTallies_KeepCausesWhenTheyFit: with room to spare the live
// tallies keep their cause lines.
func TestLiveTallies_KeepCausesWhenTheyFit(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, settled(causedItems(1)), 20, testLiveStyle)
	if frame := b.String(); !strings.Contains(frame, "lockfile pins item-0 (+1 more)") {
		t.Fatalf("live frame dropped causes it had room for:\n%s", frame)
	}
}
