package core

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

var allOutcomes = []record.Outcome{
	record.OutcomeSucceeded, record.OutcomeSatisfied, record.OutcomeExcluded,
	record.OutcomeRefused, record.OutcomeFailed, record.OutcomeCancelled,
}

func TestEntityStateOfMapsEveryOutcomeToItsLegacyState(t *testing.T) {
	want := map[record.Outcome]EntityState{
		record.OutcomeSucceeded: Done,
		record.OutcomeSatisfied: Done,
		record.OutcomeExcluded:  Skipped,
		record.OutcomeRefused:   Blocked,
		record.OutcomeFailed:    Failed,
		record.OutcomeCancelled: Cancelled,
	}
	for _, outcome := range allOutcomes {
		if got := EntityStateOf(record.PhaseSettled, outcome); got != want[outcome] {
			t.Errorf("EntityStateOf(settled, %s) = %q, want %q", outcome, got, want[outcome])
		}
	}
}

func TestEntityStateOfIgnoresOutcomeBeforeSettling(t *testing.T) {
	for _, outcome := range allOutcomes {
		if got := EntityStateOf(record.PhasePending, outcome); got != Pending {
			t.Errorf("EntityStateOf(pending, %s) = %q, want %q", outcome, got, Pending)
		}
		if got := EntityStateOf(record.PhaseRunning, outcome); got != Running {
			t.Errorf("EntityStateOf(running, %s) = %q, want %q", outcome, got, Running)
		}
	}
}

func TestEntityStateOfRefusesAnUnknownOutcome(t *testing.T) {
	if got := EntityStateOf(record.PhaseSettled, "invented"); got != "" {
		t.Errorf("EntityStateOf(settled, invented) = %q, want the zero state", got)
	}
	if got := EntityStateOf("invented", record.OutcomeFailed); got != "" {
		t.Errorf("EntityStateOf(invented, failed) = %q, want the zero state", got)
	}
}

func TestResolutionOfKeepsTodaysThreeReasons(t *testing.T) {
	cases := []struct {
		outcome   record.Outcome
		defineRan bool
		want      Resolution
	}{
		{record.OutcomeSucceeded, true, ResolutionExecuted},
		{record.OutcomeSucceeded, false, ResolutionNoWork},
		{record.OutcomeSatisfied, false, ResolutionAlreadySatisfied},
		{record.OutcomeSatisfied, true, ResolutionAlreadySatisfied},
		{record.OutcomeExcluded, false, ResolutionNoWork},
		{record.OutcomeRefused, false, ResolutionNoWork},
		{record.OutcomeFailed, true, ResolutionNoWork},
		{record.OutcomeCancelled, false, ResolutionNoWork},
	}
	for _, c := range cases {
		if got := ResolutionOf(c.outcome, c.defineRan); got != c.want {
			t.Errorf("ResolutionOf(%s, defineRan=%t) = %q, want %q", c.outcome, c.defineRan, got, c.want)
		}
	}
}

// TestConclusionStateOfAgreesWithInferConclusion proves the derived headline
// is the one InferConclusion already computes, for a run whose Tasks all end
// as worst, over every combination of changed, planned and dryRun.
func TestConclusionStateOfAgreesWithInferConclusion(t *testing.T) {
	for _, worst := range allOutcomes {
		for _, changed := range []bool{false, true} {
			for _, planned := range []bool{false, true} {
				for _, dryRun := range []bool{false, true} {
					snap := Snapshot{
						Tasks:  []TaskSnapshot{{State: EntityStateOf(record.PhaseSettled, worst)}},
						DryRun: dryRun,
					}
					if changed {
						snap.Changes = []ChangesSnapshot{{Records: []EffectRecord{{Verb: "created", Object: "file"}}}}
					}
					if planned {
						snap.Plans = []PlanSnapshot{{Records: []EffectRecord{{Verb: "create", Object: "file"}}}}
					}
					want := InferConclusion(snap).State
					if got := ConclusionStateOf(worst, changed, planned, dryRun); got != want {
						t.Errorf("worst=%s changed=%t planned=%t dryRun=%t: ConclusionStateOf = %q, InferConclusion = %q",
							worst, changed, planned, dryRun, got, want)
					}
				}
			}
		}
	}
}
