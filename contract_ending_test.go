package evo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// endingCase is one way a run ends and the verdict CONTRACT.md §4 gives it.
type endingCase struct {
	name     string
	run      func(t *testing.T, mode evo.Output) contractRun
	want     evo.Outcome
	wantExit int
}

// bodyReturns runs a body that declares nothing and returns err.
func bodyReturns(err error) func(*testing.T, evo.Output) contractRun {
	return func(t *testing.T, mode evo.Output) contractRun {
		return executeContract(context.Background(), t, evo.Config{Name: "ending", Output: mode},
			func(context.Context, *evo.Run) error { return err })
	}
}

// taskReturns runs one Task whose Define returns err.
func taskReturns(err error) func(*testing.T, evo.Output) contractRun {
	return singleTask(nil, func(context.Context) error { return err })
}

func endingCases() []endingCase {
	already := func(context.Context) (bool, error) { return true, nil }
	succeed := func(context.Context) error { return nil }
	return []endingCase{
		{"nil", taskReturns(nil), evo.Succeeded, 0},
		{"verify holds", singleTask(already, succeed), evo.Satisfied, 0},
		{"Exclude", taskReturns(evo.Exclude("protected")), evo.Excluded, 0},
		{"wrapped Exclude", taskReturns(fmt.Errorf("policy: %w", evo.Exclude("protected"))), evo.Excluded, 0},
		{"Refuse", taskReturns(evo.Refuse("no remote")), evo.Refused, 1},
		{"wrapped Refuse", taskReturns(fmt.Errorf("gate: %w", evo.Refuse("no remote"))), evo.Refused, 1},
		{"body Refuse", bodyReturns(evo.Refuse("no remote")), evo.Refused, 1},
		{"Fail", taskReturns(evo.Fail("lint failed")), evo.Outcome(evo.Failed), 2},
		{"error", taskReturns(errors.New("boom")), evo.Outcome(evo.Failed), 2},
		{"body error", bodyReturns(errors.New("boom")), evo.Outcome(evo.Failed), 2},
		{"cancelled", cancelledWhileRunning, evo.Outcome(evo.Cancelled), 130},
	}
}

// TestReturnedEndingSettlesItsOutcome pins CONTRACT.md §4: the value a
// Define callback or the run body returns settles the Outcome, wrapped or
// not, and the exit code follows the Outcome in every Output mode.
func TestReturnedEndingSettlesItsOutcome(t *testing.T) {
	for _, tc := range endingCases() {
		for _, mode := range contractOutputs {
			t.Run(tc.name+"/"+outputName[mode], func(t *testing.T) {
				run := tc.run(t, mode)
				if got := run.Result.Conclusion.Outcome; got != tc.want {
					t.Errorf("Conclusion.Outcome = %q, want %q", got, tc.want)
				}
				if got := run.Result.ExitCode(); got != tc.wantExit {
					t.Errorf("Result.ExitCode() = %d, want %d", got, tc.wantExit)
				}
				switch mode {
				case evo.JSON:
					if got := runDocument(t, run.Stdout)["exit_code"]; got != float64(tc.wantExit) {
						t.Errorf("evo.run exit_code = %v, want %d", got, tc.wantExit)
					}
				case evo.JSONL:
					for _, event := range eventsOfType(runEvents(t, run.Stdout), "run.finished") {
						if got := payload(event)["exit_code"]; got != float64(tc.wantExit) {
							t.Errorf("run.finished exit_code = %v, want %d", got, tc.wantExit)
						}
					}
				}
			})
		}
	}
}
