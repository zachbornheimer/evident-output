package evo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// explanationCase is one run and the text its Explanation must hold. An
// empty mention means the Explanation must be empty; anyText means it must
// say something, whatever the words.
type explanationCase struct {
	name    string
	run     func(t *testing.T, mode evo.Output) contractRun
	mention string
}

const anyText = "\x00any"

// groupReturns runs one Group whose child Tasks return errs, in order.
func groupReturns(errs ...error) func(*testing.T, evo.Output) contractRun {
	return func(t *testing.T, mode evo.Output) contractRun {
		return executeContract(context.Background(), t, evo.Config{Name: "explanation", Output: mode},
			func(_ context.Context, r *evo.Run) error {
				g := r.Group("checks")
				for i, err := range errs {
					g.Task(string(rune('a' + i))).Define(func(context.Context) error { return err })
				}
				return nil
			})
	}
}

func explanationCases() []explanationCase {
	already := func(context.Context) (bool, error) { return true, nil }
	succeed := func(context.Context) error { return nil }
	return []explanationCase{
		{"succeeded", taskReturns(nil), ""},
		{"satisfied", singleTask(already, succeed), ""},
		{"excluded", taskReturns(evo.Exclude("protected")), ""},
		{"refused task", taskReturns(evo.Refuse("no remote")), "no remote"},
		{"refused body", bodyReturns(evo.Refuse("no remote")), "no remote"},
		{"failed Fail", taskReturns(evo.Fail("lint failed")), "lint failed"},
		{"failed error", taskReturns(errors.New("disk full")), "disk full"},
		{"failed body", bodyReturns(errors.New("disk full")), "disk full"},
		{"refused beside succeeded", groupReturns(nil, evo.Refuse("no remote")), "no remote"},
		{"failed beside refused", groupReturns(evo.Refuse("no remote"), errors.New("disk full")), "disk full"},
		{"cancelled", cancelledWhileRunning, anyText},
	}
}

// TestConclusionExplainsTheDecidingEnding pins CONTRACT.md §1: when a run is
// Refused, Failed or Cancelled, Conclusion.Explanation is the deciding Task's
// ending, and evo.run and run.finished carry the same text; otherwise it is
// empty.
func TestConclusionExplainsTheDecidingEnding(t *testing.T) {
	for _, tc := range explanationCases() {
		for _, mode := range contractOutputs {
			t.Run(tc.name+"/"+outputName[mode], func(t *testing.T) {
				run := tc.run(t, mode)
				got := run.Result.Conclusion.Explanation
				assertExplanation(t, "Conclusion.Explanation", got, tc.mention)
				switch mode {
				case evo.JSON:
					assertWireExplanation(t, "evo.run explanation", runDocument(t, run.Stdout)["explanation"], got)
				case evo.JSONL:
					for _, event := range eventsOfType(runEvents(t, run.Stdout), "run.finished") {
						assertWireExplanation(t, "run.finished explanation", payload(event)["explanation"], got)
					}
				default:
					if got != "" && !strings.Contains(run.Text(), got) {
						t.Errorf("text does not show the Explanation %q:\n%s", got, run.Text())
					}
				}
			})
		}
	}
}

func assertExplanation(t *testing.T, what, got, mention string) {
	t.Helper()
	switch mention {
	case "":
		if got != "" {
			t.Errorf("%s = %q, want empty", what, got)
		}
	case anyText:
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s is empty, want what interrupted the run", what)
		}
	default:
		if !strings.Contains(got, mention) {
			t.Errorf("%s = %q, want it to name %q", what, got, mention)
		}
	}
}

// assertWireExplanation checks a machine projection carries exactly the
// Conclusion's Explanation; an absent field stands for an empty one.
func assertWireExplanation(t *testing.T, what string, wire any, want string) {
	t.Helper()
	if got, _ := wire.(string); got != want {
		t.Errorf("%s = %v, want %q", what, wire, want)
	}
}

// TestExplanationNamesTheEndingNotTheFirstProblem pins CONTRACT.md §1: a
// Task that records a non-fatal Problem and then fails explains the run with
// its ending, never with the Problem it recorded first.
func TestExplanationNamesTheEndingNotTheFirstProblem(t *testing.T) {
	const ending, nit = "lint failed", "style nit"
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "explanation", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					r.Task("lint").Define(func(context.Context) error { return evo.Fail(ending) }).Problem(nit)
					return nil
				})
			got := run.Result.Conclusion.Explanation
			if got != ending {
				t.Fatalf("Conclusion.Explanation = %q, want the ending %q", got, ending)
			}
			switch mode {
			case evo.JSON:
				assertWireExplanation(t, "evo.run explanation", runDocument(t, run.Stdout)["explanation"], got)
			case evo.JSONL:
				for _, event := range eventsOfType(runEvents(t, run.Stdout), "run.finished") {
					assertWireExplanation(t, "run.finished explanation", payload(event)["explanation"], got)
				}
			}
		})
	}
}
