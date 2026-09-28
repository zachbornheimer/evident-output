package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
)

// TestProblem_TenPlusIndependentFindings is the ZYS-848 acceptance item
// "one Task can retain 10+ Problems with independent locations/codes" —
// exercised through the real public API (TaskHandle.Problem), not a
// hand-built Snapshot. Mirrors zq's reportFileIntegrityIssues shape: one
// owning Task, one Problem call per finding.
func TestProblem_TenPlusIndependentFindings(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("file integrity")
	const n = 12
	for i := range n {
		task.Problem(fmt.Sprintf("checksum mismatch %d", i),
			evo.On(fmt.Sprintf("file-%d.bin", i)),
			evo.Code(fmt.Sprintf("FILE-%03d", i)),
			evo.Location(fmt.Sprintf("file-%d.bin", i), i+1, 0),
		)
	}
	task.Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	got := snap.Tasks[0]
	if got.State != evo.Failed {
		t.Fatalf("want Failed (accumulated blocking Problems override a nil Define return), got %v", got.State)
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
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("golangci findings")
	for i := range 15 {
		task.Problem(fmt.Sprintf("finding %d", i), evo.Code("LINT001"))
	}
	task.Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	if len(snap.Tasks) != 1 {
		t.Fatalf("want exactly one Task entity regardless of Problem count, got %d: %#v", len(snap.Tasks), snap.Tasks)
	}
	if len(snap.Tasks[0].Problems) != 15 {
		t.Fatalf("want 15 Problems on the one Task, got %d", len(snap.Tasks[0].Problems))
	}
}

// TestProblem_DoneWithAccumulatedProblemsPromotesToFailed is the Decisions
// (2026-09-23) contract's central rule: "if a Define callback returns nil
// but accumulated at least one blocking Problem, the Task resolves Failed."
func TestProblem_DoneWithAccumulatedProblemsPromotesToFailed(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("policy check")
	task.Problem("rule XYZ violated", evo.Code("XYZ"))
	task.Define(func(context.Context) error { return nil }) // claims success
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	if snap.Tasks[0].State != evo.Failed {
		t.Fatalf("accumulated Problem must override a nil-error Define, want Failed, got %v", snap.Tasks[0].State)
	}
}

// TestProblem_BareDoneWithAccumulatedProblemsPromotesToFailed covers the
// same promotion for a non-Define, hand-resolved task (a bare task.Done()
// after accumulating Problems) — the rule is not special-cased to Define.
func TestProblem_BareDoneWithAccumulatedProblemsPromotesToFailed(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("manual check")
	task.Problem("finding one", evo.Code("A"))
	task.Problem("finding two", evo.Code("B"))
	succeed(task, "looks fine")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	if snap.Tasks[0].State != evo.Failed {
		t.Fatalf("want Failed, got %v", snap.Tasks[0].State)
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
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	out.Task("clean check").Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	if snap.Tasks[0].State != evo.Done {
		t.Fatalf("want Done, got %v", snap.Tasks[0].State)
	}
}

// TestProblem_MergesWithExplicitFail proves accumulated Problems are never
// dropped when the caller finally resolves with its own Fail: both the
// accumulated findings and the terminal Fail's own Problem survive, in
// call order.
func TestProblem_MergesWithExplicitFail(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	task.Problem("warning-level finding", evo.Code("W1"))
	task.Fail("build failed", evo.Code("BUILD-FAIL"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
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
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	out.Task("policy").
		Problem("missing header", evo.Code("HDR"), evo.Next(evo.Label("add license header"))).
		Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
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
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("audit")
	const n = 9 // > maxVisibleProblems (5), so plain output bounds it
	for i := range n {
		task.Problem(fmt.Sprintf("finding %d", i), evo.Code(fmt.Sprintf("A%d", i)))
	}
	task.Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	encoded, err := render.EncodeJSON(snap)
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
	plain, err := evo.RenderPlain(snap, evo.PlainOptions{NoColor: true})
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
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("cache")
	task.Problem("stale entry ignored", evo.Severity(evo.SeverityWarning),
		evo.Detail("cache/entry-42.json is 9 days old"),
		evo.Code("CACHE-001"),
		evo.Location("cache/entry-42.json", 0, 0),
	)
	succeed(task)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
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

// TestWarn_ReturnsHandleForChaining proves Warn's new *TaskHandle return
// chains like Next/NextCommand already do.
func TestWarn_ReturnsHandleForChaining(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })

	succeed(out.Task("chain").Problem("heads up", evo.Severity(evo.SeverityWarning)), "finished anyway")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "finished anyway") {
		t.Fatalf("want chained Done to take effect:\n%s", buf.String())
	}
}

// finishProblemRun settles a run built by declare and returns the one
// Task's snapshot plus the human transcript.
func finishProblemRun(t *testing.T, declare func(out *evo.Output)) (evo.TaskSnapshot, string) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })
	declare(out)
	_ = out.Finish()
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 {
		t.Fatalf("want one Task, got %d: %#v", len(snap.Tasks), snap.Tasks)
	}
	return snap.Tasks[0], buf.String()
}

func assertProblemFailsTask(t *testing.T, got evo.TaskSnapshot, human string) {
	t.Helper()
	if got.State != evo.Failed {
		t.Fatalf("a Task holding a blocking Problem must settle Failed, got %v\n%s", got.State, human)
	}
	if len(got.Problems) != 1 || got.Problems[0].Summary != "blocking finding" {
		t.Fatalf("the Problem must survive to the Snapshot, got %#v", got.Problems)
	}
	if !strings.Contains(human, "blocking finding") {
		t.Fatalf("the Problem must reach human output:\n%s", human)
	}
}

// TestProblem_SkippedCannotLaunderAProblem: Skipped is a success-class
// outcome, so a Problem recorded before it still fails the Task.
func TestProblem_SkippedCannotLaunderAProblem(t *testing.T) {
	got, human := finishProblemRun(t, func(out *evo.Output) {
		task := out.Task("scan")
		task.Define(func(context.Context) error {
			task.Problem("blocking finding")
			task.Skipped(evo.Reason("nothing to do"))
			return nil
		})
	})
	assertProblemFailsTask(t, got, human)
}

// TestProblem_UnresolvedWarnedTaskKeepsItsProblem: Finish's amnesty for a
// warned-but-unresolved Task must not settle it Done over a Problem.
func TestProblem_UnresolvedWarnedTaskKeepsItsProblem(t *testing.T) {
	got, human := finishProblemRun(t, func(out *evo.Output) {
		out.Task("scan").Problem("blocking finding").Problem("also a warning", evo.Severity(evo.SeverityWarning))
	})
	assertProblemFailsTask(t, got, human)
}

// TestProblem_UnresolvedTaskKeepsItsProblem: a Task that recorded a
// Problem and was never resolved still fails; the Problem is its story.
func TestProblem_UnresolvedTaskKeepsItsProblem(t *testing.T) {
	got, human := finishProblemRun(t, func(out *evo.Output) {
		out.Task("scan").Problem("blocking finding")
	})
	assertProblemFailsTask(t, got, human)
}

// TestProblem_VisibleInSnapshotWhileRunning: every Problem is kept in the
// Snapshot from the moment it is recorded, not only after the Task settles.
func TestProblem_VisibleInSnapshotWhileRunning(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("scan")
	var live evo.TaskSnapshot
	task.Define(func(context.Context) error {
		task.Problem("blocking finding")
		live = task.Snapshot()
		return nil
	})
	_ = out.Finish()
	if len(live.Problems) != 1 || live.Problems[0].Summary != "blocking finding" {
		t.Fatalf("a running Task's Snapshot must carry its recorded Problem, got %#v", live.Problems)
	}
}
