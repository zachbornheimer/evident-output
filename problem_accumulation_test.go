package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

// TestProblem_TenPlusIndependentFindings is the ZYS-848 acceptance item
// "one Task can retain 10+ Problems with independent locations/codes" —
// exercised through the real public API (Task.Problem), not a
// hand-built Snapshot. Mirrors zq's reportFileIntegrityIssues shape: one
// owning Task, one Problem call per finding.
func TestProblem_TenPlusIndependentFindings(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("file integrity")
	const n = 12
	for i := range n {
		task.Problem(fmt.Sprintf("checksum mismatch %d", i),
			On(fmt.Sprintf("file-%d.bin", i)),
			Code(fmt.Sprintf("FILE-%03d", i)),
			location(fmt.Sprintf("file-%d.bin", i), i+1, 0),
		)
	}
	task.Define(func(context.Context) error { return nil })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	got := snap.Tasks[0]
	if got.State != succeeded {
		t.Fatalf("want Done (Problems never end a Task), got %v", got.State)
	}
	if len(got.Problems) != n {
		t.Fatalf("want %d retained Problems, got %d: %#v", n, len(got.Problems), got.Problems)
	}
	for i, p := range got.Problems {
		wantCode := fmt.Sprintf("FILE-%03d", i)
		if p.Code != wantCode {
			t.Fatalf("problem %d: want code %s, got %s", i, wantCode, p.Code)
		}
		if p.Location == nil || p.Location.Line != i+1 {
			t.Fatalf("problem %d: want independent Location.Line=%d, got %#v", i, i+1, p.Location)
		}
	}
}

// TestProblem_ResolvesTaskOnceNotChildTasks is the acceptance item "the
// Task resolves once; Problems do not become child Tasks" — the run's own
// entity count must stay at exactly one Task regardless of how many
// Problems were accumulated on it.
func TestProblem_ResolvesTaskOnceNotChildTasks(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("golangci findings")
	for i := range 15 {
		task.Problem(fmt.Sprintf("finding %d", i), Code("LINT001"))
	}
	task.Define(func(context.Context) error { return nil })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	if len(snap.Tasks) != 1 {
		t.Fatalf("want exactly one Task entity regardless of Problem count, got %d: %#v", len(snap.Tasks), snap.Tasks)
	}
	if len(snap.Tasks[0].Problems) != 15 {
		t.Fatalf("want 15 Problems on the one Task, got %d", len(snap.Tasks[0].Problems))
	}
}

// TestProblem_DoneWithAccumulatedProblemsStaysDone is CONTRACT.md section 4:
// a Define callback that returns nil settles Done however many Problems it
// recorded; only a returned Fail ends a Task Failed.
func TestProblem_DoneWithAccumulatedProblemsStaysDone(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("policy check")
	task.Problem("rule XYZ violated", Code("XYZ"))
	task.Define(func(context.Context) error { return nil }) // claims success
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	if snap.Tasks[0].State != succeeded {
		t.Fatalf("a Problem must not override a nil-error Define, want Done, got %v", snap.Tasks[0].State)
	}
}

// TestProblem_BareDoneWithAccumulatedProblemsStaysDone covers the same rule
// for a non-Define, hand-resolved task (a bare task.Done() after
// accumulating Problems).
func TestProblem_BareDoneWithAccumulatedProblemsStaysDone(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("manual check")
	task.Problem("finding one", Code("A"))
	task.Problem("finding two", Code("B"))
	succeed(task, "looks fine")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	if snap.Tasks[0].State != succeeded {
		t.Fatalf("want Done, got %v", snap.Tasks[0].State)
	}
	if len(snap.Tasks[0].Problems) != 2 {
		t.Fatalf("want both accumulated Problems retained, got %d", len(snap.Tasks[0].Problems))
	}
}

// TestProblem_NoAccumulationLeavesDoneUntouched proves Problem's promotion
// is opt-in per accumulation: a Task with zero accumulated Problems still
// resolves Done normally.
func TestProblem_NoAccumulationLeavesDoneUntouched(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	out.Task("clean check").Define(func(context.Context) error { return nil })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	if snap.Tasks[0].State != succeeded {
		t.Fatalf("want Done, got %v", snap.Tasks[0].State)
	}
}

// TestProblem_MergesWithExplicitFail proves accumulated Problems are never
// dropped when the caller finally resolves with its own Fail: both the
// accumulated findings and the terminal Fail's own Problem survive, in
// call order.
func TestProblem_MergesWithExplicitFail(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("build")
	task.Problem("warning-level finding", Code("W1"))
	task.fail("build failed", Code("BUILD-FAIL"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	got := snap.Tasks[0].Problems
	if len(got) != 2 {
		t.Fatalf("want 2 Problems (1 accumulated + Fail's own), got %d: %#v", len(got), got)
	}
	if got[0].Code != "W1" || got[1].Code != "BUILD-FAIL" {
		t.Fatalf("want accumulated Problem first, Fail's own last, got %#v", got)
	}
}

// TestProblem_RemediesSurviveProjection is the acceptance item "remedies/
// actions survive projection" — a Next(...) action attached to an
// accumulated Problem must still render on the plain human view.
func TestProblem_RemediesSurviveProjection(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	out.Task("policy").
		Problem("missing header", Code("HDR"), Next(label("add license header"))).
		Define(func(context.Context) error { return nil })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "add license header") {
		t.Fatalf("want remedy rendered from an accumulated Problem:\n%s", got)
	}
}

// TestProblem_SurvivesInJSON is the acceptance item "all Problems remain
// available in Snapshot/JSON/JSONL" for the JSON projection specifically —
// bounded human plain output must not shrink the JSON encoding.
func TestProblem_SurvivesInJSON(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("audit")
	const n = 9 // > maxVisibleProblems (5), so plain output bounds it
	for i := range n {
		task.Problem(fmt.Sprintf("finding %d", i), Code(fmt.Sprintf("A%d", i)))
	}
	task.Define(func(context.Context) error { return nil })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	encoded, err := wire.EncodeJSON(snap)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var decoded struct {
		Tasks []struct {
			Problems []struct {
				Code string `json:"code"`
			} `json:"problems"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(decoded.Tasks) != 1 || len(decoded.Tasks[0].Problems) != n {
		t.Fatalf("want %d Problems in JSON (unbounded), got %#v", n, decoded.Tasks)
	}

	// Plain projection of the same Snapshot bounds display and still states
	// the true count via the omission line (count remains authoritative).
	plain, err := renderPlain_1(snap, plainOptions{noColor: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), "and 4 more failures") {
		t.Fatalf("want bounded plain view with omission count, got:\n%s", plain)
	}
}

// TestWarn_AcceptsStructuredProblemOptions is the acceptance item "warning
// Problems can carry the same useful structured metadata where
// appropriate" — Warn now takes Detail/Code/Location like Problem/Fail/
// Block do, and the metadata is retained on the Snapshot's Warnings.
func TestWarn_AcceptsStructuredProblemOptions(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("cache")
	task.Problem("stale entry ignored", severity(severityWarning),
		Detail("cache/entry-42.json is 9 days old"),
		Code("CACHE-001"),
		location("cache/entry-42.json", 0, 0),
	)
	succeed(task)
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.snapshot()
	warnings := snap.Tasks[0].Warnings
	if len(warnings) != 1 {
		t.Fatalf("want one warning, got %d", len(warnings))
	}
	w := warnings[0]
	if w.Code != "CACHE-001" || w.Detail != "cache/entry-42.json is 9 days old" {
		t.Fatalf("want structured warning metadata retained, got %#v", w)
	}
	if w.Location == nil || w.Location.Path != "cache/entry-42.json" {
		t.Fatalf("want warning Location retained, got %#v", w.Location)
	}
}

// TestWarn_ReturnsHandleForChaining proves Warn's new *Task return
// chains like Next/NextCommand already do.
func TestWarn_ReturnsHandleForChaining(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Task("chain").Problem("heads up", severity(severityWarning)), "finished anyway")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "finished anyway") {
		t.Fatalf("want chained Done to take effect:\n%s", buf.String())
	}
}

// finishProblemRun settles a run built by declare and returns the one
// Task's snapshot plus the human transcript.
func finishProblemRun(t *testing.T, declare func(out *Run)) (taskSnapshot, string) {
	t.Helper()
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })
	declare(out)
	_ = out.finish()
	snap := out.snapshot()
	if len(snap.Tasks) != 1 {
		t.Fatalf("want one Task, got %d: %#v", len(snap.Tasks), snap.Tasks)
	}
	return snap.Tasks[0], buf.String()
}

func assertProblemKeptOnTask(t *testing.T, got taskSnapshot, want entityState, human string) {
	t.Helper()
	if got.State != want {
		t.Fatalf("a Problem must not change the Task's outcome: want %v, got %v\n%s", want, got.State, human)
	}
	if len(got.Problems) != 1 || got.Problems[0].Summary != "blocking finding" {
		t.Fatalf("the Problem must survive to the Snapshot, got %#v", got.Problems)
	}
	if !strings.Contains(human, "blocking finding") {
		t.Fatalf("the Problem must reach human output:\n%s", human)
	}
}

// TestProblem_UnresolvedWarnedTaskKeepsItsProblem: Finish's amnesty for a
// warned-but-unresolved Task settles it Done with its Problem attached.
func TestProblem_UnresolvedWarnedTaskKeepsItsProblem(t *testing.T) {
	got, human := finishProblemRun(t, func(out *Run) {
		out.Task("scan").Problem("blocking finding").Problem("also a warning", severity(severityWarning))
	})
	assertProblemKeptOnTask(t, got, succeeded, human)
}

// TestProblem_UnresolvedTaskKeepsItsProblem: a Task that recorded a
// Problem and was never resolved settles Done; the Problem stays attached.
func TestProblem_UnresolvedTaskKeepsItsProblem(t *testing.T) {
	got, human := finishProblemRun(t, func(out *Run) {
		out.Task("scan").Problem("blocking finding")
	})
	assertProblemKeptOnTask(t, got, succeeded, human)
}

// TestProblem_VisibleInSnapshotWhileRunning: every Problem is kept in the
// Snapshot from the moment it is recorded, not only after the Task settles.
func TestProblem_VisibleInSnapshotWhileRunning(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("scan")
	var live taskSnapshot
	task.Define(func(context.Context) error {
		task.Problem("blocking finding")
		live = task.snapshot()
		return nil
	})
	_ = out.finish()
	if len(live.Problems) != 1 || live.Problems[0].Summary != "blocking finding" {
		t.Fatalf("a running Task's Snapshot must carry its recorded Problem, got %#v", live.Problems)
	}
}
