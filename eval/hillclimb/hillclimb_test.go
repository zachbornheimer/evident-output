package hillclimb_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/eval/hillclimb"
	"github.com/zachbornheimer/evident-output/eval/scoreboard"
)

func standing(k int, passes map[string]int) hillclimb.Standing {
	return hillclimb.Standing{SamplesPerTask: k, Passes: passes}
}

func TestAccept_RequiresGainOfAtLeastOneSample(t *testing.T) {
	before := standing(3, map[string]int{"A": 1, "B": 2})
	if d := hillclimb.Accept(before, standing(3, map[string]int{"A": 1, "B": 2})); d.Accepted {
		t.Errorf("no gain must be rejected: %+v", d)
	}
	if d := hillclimb.Accept(before, standing(3, map[string]int{"A": 2, "B": 2})); !d.Accepted {
		t.Errorf("a one-sample gain must be accepted: %+v", d)
	}
}

func TestAccept_RejectsATaskDroppingTwoSamplesEvenWithNetGain(t *testing.T) {
	before := standing(3, map[string]int{"A": 3, "B": 0})
	after := standing(3, map[string]int{"A": 1, "B": 3}) // net +1, A fell by 2
	if d := hillclimb.Accept(before, after); d.Accepted || !strings.Contains(d.Reason, "A") {
		t.Errorf("a 2-sample drop must reject: %+v", d)
	}
	oneDrop := standing(3, map[string]int{"A": 2, "B": 3}) // net +2, A fell by 1
	if d := hillclimb.Accept(before, oneDrop); !d.Accepted {
		t.Errorf("a 1-sample drop is tolerated: %+v", d)
	}
}

func TestStanding_SaturatedMeansEverySampleOfEveryTaskPassed(t *testing.T) {
	if !standing(3, map[string]int{"A": 3, "B": 3}).Saturated() {
		t.Error("all passing must be saturated")
	}
	if standing(3, map[string]int{"A": 3, "B": 2}).Saturated() {
		t.Error("one failure must not be saturated")
	}
	if standing(3, nil).Saturated() {
		t.Error("no tasks is not saturated")
	}
}

func TestCheckPaths_FrozenTestAndOutOfSurfacePathsAreViolations(t *testing.T) {
	frozen := []string{
		"internal/agent/evaltask/testdata/tasks/T1-zq-prune/prompt.md",
		"internal/agent/evaltask/testdata/tasks/T1-zq-prune/expect.json",
		"internal/agent/evaltask/grade.go",
		"conformance/spec/ratchet.json",
		"testdata/api_golden.txt",
		"testdata/api_golden_future.txt",
		"eval/hillclimb/guard.go",
	}
	for _, p := range frozen {
		if v := hillclimb.CheckPaths([]string{p}); len(v) != 1 || !strings.Contains(v[0], "frozen") {
			t.Errorf("%s: want one frozen violation, got %v", p, v)
		}
	}
	if v := hillclimb.CheckPaths([]string{"internal/agent/rules/rules_api_test.go"}); len(v) != 1 {
		t.Errorf("a test file must be a violation: %v", v)
	}
	if v := hillclimb.CheckPaths([]string{"task.go"}); len(v) != 1 || !strings.Contains(v[0], "outside") {
		t.Errorf("scheduler code is outside the tuned surface: %v", v)
	}
	for _, ok := range []string{"docs/guides/tasks.md", "docs/reference.md", "internal/agent/rules/rules_api.go", "internal/agent/review/review.go"} {
		if v := hillclimb.CheckPaths([]string{ok}); len(v) != 0 {
			t.Errorf("%s is tuned surface, got %v", ok, v)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"a/**", "a/b/c.txt", true},
		{"a/**", "a", true},
		{"a/**/z", "a/b/c/z", true},
		{"a/*.go", "a/b/c.go", false},
		{"testdata/api_golden*.txt", "testdata/api_golden_x.txt", true},
		{"docs/reference.md", "docs/reference.md.bak", false},
	}
	for _, tc := range cases {
		if got := hillclimb.MatchGlob(tc.pattern, tc.path); got != tc.want {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestStringLiteralsOnly(t *testing.T) {
	const base = "package p\n\n// old comment\nfunc f() string { return \"old message\" }\n"
	reworded := "package p\n\n// new comment\nfunc f() string { return \"a different message\" }\n"
	logic := "package p\n\nfunc f() string { return \"old message\" + \"!\" }\n"
	if same, err := hillclimb.StringLiteralsOnly([]byte(base), []byte(reworded)); err != nil || !same {
		t.Errorf("reworded strings and comments must pass: %v %v", same, err)
	}
	if same, err := hillclimb.StringLiteralsOnly([]byte(base), []byte(logic)); err != nil || same {
		t.Errorf("added code must fail: %v %v", same, err)
	}
	if _, err := hillclimb.StringLiteralsOnly([]byte(base), []byte("package p\nfunc (")); err == nil {
		t.Error("unparseable source must be an error")
	}
}

func TestLintNouns_RejectsAnEditWithDistinctiveTaskWords(t *testing.T) {
	prompts := []string{"I'm writing the prune command; delete landed branches and worktrees.", "Write a homelab converge step for containers."}
	vocabulary := "A task is declared with Task; use Group and Sequence. Write results."
	nouns := hillclimb.DistinctiveNouns(prompts, vocabulary)
	for _, want := range []string{"prune", "homelab", "worktrees", "containers"} {
		if !slices.Contains(nouns, want) {
			t.Errorf("nouns %v lack %q", nouns, want)
		}
	}
	for _, generic := range []string{"task", "group", "write"} {
		if slices.Contains(nouns, generic) {
			t.Errorf("%q is already docs vocabulary or plain, not distinctive", generic)
		}
	}
	if found := hillclimb.LintNouns("Declare PruneLanded as a Task", nouns); !slices.Contains(found, "prune") {
		t.Errorf("a noun inside a camelCase identifier must be caught: %v", found)
	}
	if found := hillclimb.LintNouns("Declare each Task with Define", nouns); len(found) != 0 {
		t.Errorf("a generic edit must pass: %v", found)
	}
}

func TestGuard_CombinesPathsLiteralsAndNouns(t *testing.T) {
	guard := hillclimb.Guard{Nouns: []string{"prune"}}
	clean := hillclimb.Edit{Changed: []string{"docs/guides/tasks.md"}, Added: "Prefer Compute for values."}
	if v := guard.Check(clean); len(v) != 0 {
		t.Errorf("clean edit rejected: %v", v)
	}
	dirty := hillclimb.Edit{
		Changed: []string{"internal/agent/rules/rules_api.go", "task.go"},
		Added:   "prune example",
		Files: map[string]hillclimb.FilePair{
			"internal/agent/rules/rules_api.go": {Before: []byte("package p\nvar a = 1\n"), After: []byte("package p\nvar a = 2\n")},
		},
	}
	if v := guard.Check(dirty); len(v) != 3 {
		t.Errorf("want 3 violations (outside surface, code change, noun), got %v", v)
	}
}

// fakes for the loop

type fakeWorker struct{ calls int }

func (w *fakeWorker) ProposeEdit(context.Context, []hillclimb.Failure) error { w.calls++; return nil }

type fakeWorkspace struct {
	edit             hillclimb.Edit
	reverts, commits int
}

func (w *fakeWorkspace) Edit() (hillclimb.Edit, error) { return w.edit, nil }
func (w *fakeWorkspace) Revert() error                 { w.reverts++; return nil }
func (w *fakeWorkspace) Commit(string) error           { w.commits++; return nil }

type fakeChecker struct{ err error }

func (c fakeChecker) Run(context.Context) error { return c.err }

type fakeEvaluator struct {
	training []hillclimb.Standing
	heldOut  []float64
	trainAt  int
	heldAt   int
}

func (e *fakeEvaluator) Training(context.Context) (hillclimb.Standing, error) {
	s := e.training[min(e.trainAt, len(e.training)-1)]
	e.trainAt++
	return s, nil
}

func (e *fakeEvaluator) HeldOut(context.Context) (scoreboard.Aggregate, float64, error) {
	rate := e.heldOut[min(e.heldAt, len(e.heldOut)-1)]
	e.heldAt++
	return scoreboard.Aggregate{PassAt1: rate}, 0, nil
}

func newLoop(eval *fakeEvaluator, ws *fakeWorkspace, checker fakeChecker) (hillclimb.Loop, *fakeWorker) {
	worker := &fakeWorker{}
	return hillclimb.Loop{
		Worker: worker, Workspace: ws, Checker: checker, Evaluator: eval,
		Guard: hillclimb.Guard{}, MaxUSD: 100, MaxSteps: 20,
	}, worker
}

func okEdit() hillclimb.Edit { return hillclimb.Edit{Changed: []string{"docs/guides/tasks.md"}} }

func TestLoop_StopsWhenTrainingSaturates(t *testing.T) {
	eval := &fakeEvaluator{
		training: []hillclimb.Standing{standing(2, map[string]int{"A": 0}), standing(2, map[string]int{"A": 2})},
		heldOut:  []float64{0.5},
	}
	ws := &fakeWorkspace{edit: okEdit()}
	loop, _ := newLoop(eval, ws, fakeChecker{})
	result, err := loop.Run(context.Background())
	if err != nil || result.Stop != hillclimb.StopSaturated || ws.commits != 1 {
		t.Fatalf("result = %+v, err = %v, commits = %d", result, err, ws.commits)
	}
}

func TestLoop_StopsAfterTwoConsecutiveHeldOutChecksWithoutGain(t *testing.T) {
	eval := &fakeEvaluator{
		training: []hillclimb.Standing{
			standing(5, map[string]int{"A": 0}), standing(5, map[string]int{"A": 1}),
			standing(5, map[string]int{"A": 2}), standing(5, map[string]int{"A": 3}), standing(5, map[string]int{"A": 4}),
		},
		heldOut: []float64{0.4, 0.4, 0.3, 0.9},
	}
	ws := &fakeWorkspace{edit: okEdit()}
	loop, _ := newLoop(eval, ws, fakeChecker{})
	result, err := loop.Run(context.Background())
	if err != nil || result.Stop != hillclimb.StopHeldOut {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if eval.heldAt != 3 {
		t.Errorf("want 3 held-out checks (baseline gain, then two stale), got %d", eval.heldAt)
	}
}

func TestLoop_RejectsAndRevertsGuardCheckAndScoreFailures(t *testing.T) {
	frozen := hillclimb.Edit{Changed: []string{"internal/agent/evaltask/grade.go"}}
	cases := map[string]struct {
		edit    hillclimb.Edit
		checker fakeChecker
		after   hillclimb.Standing
		outcome string
	}{
		"frozen file":      {frozen, fakeChecker{}, standing(3, map[string]int{"A": 3}), "guard"},
		"gate failure":     {okEdit(), fakeChecker{err: errors.New("lint failed")}, standing(3, map[string]int{"A": 3}), "checks"},
		"no training gain": {okEdit(), fakeChecker{}, standing(3, map[string]int{"A": 0}), "score"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			eval := &fakeEvaluator{training: []hillclimb.Standing{standing(3, map[string]int{"A": 0}), tc.after}, heldOut: []float64{0}}
			ws := &fakeWorkspace{edit: tc.edit}
			loop, _ := newLoop(eval, ws, tc.checker)
			loop.MaxSteps = 1
			result, err := loop.Run(context.Background())
			if err != nil || result.Steps[0].Outcome != tc.outcome {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if ws.reverts != 1 || ws.commits != 0 {
				t.Errorf("rejected edit must be reverted, not committed: reverts=%d commits=%d", ws.reverts, ws.commits)
			}
		})
	}
}

func TestLoop_StopsAtDollarCap(t *testing.T) {
	costly := standing(3, map[string]int{"A": 0})
	costly.CostUSD = 60
	eval := &fakeEvaluator{training: []hillclimb.Standing{costly}, heldOut: []float64{0}}
	ws := &fakeWorkspace{edit: okEdit()}
	loop, worker := newLoop(eval, ws, fakeChecker{})
	result, err := loop.Run(context.Background())
	if err != nil || result.Stop != hillclimb.StopSpendCap {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if worker.calls != 1 {
		t.Errorf("want exactly one edit before $120 >= $100, got %d", worker.calls)
	}
}

func TestStandingFromRecords_CountsPassesAndCollectsFailures(t *testing.T) {
	records := recordsFor("A", true, false, false)
	got := hillclimb.StandingFromRecords(records)
	if got.SamplesPerTask != 3 || got.Passes["A"] != 1 || len(got.Failures) != 2 {
		t.Errorf("standing = %+v", got)
	}
}
