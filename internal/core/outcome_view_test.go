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

// snapshotOf is a run whose Tasks settled to outcomes (one Task each), with
// a warning on the first Task when warned, plus the optional ledgers.
func snapshotOf(outcomes []record.Outcome, changed, planned, dryRun, warned bool) Snapshot {
	snap := Snapshot{DryRun: dryRun}
	for i, outcome := range outcomes {
		task := TaskSnapshot{State: EntityStateOf(record.PhaseSettled, outcome)}
		if warned && i == 0 {
			task.Warnings = []Problem{{Summary: "careful"}}
		}
		snap.Tasks = append(snap.Tasks, task)
	}
	if changed {
		snap.Changes = []ChangesSnapshot{{Records: []EffectRecord{{Verb: "created", Object: "file"}}}}
	}
	if planned {
		snap.Plans = []PlanSnapshot{{Records: []EffectRecord{{Verb: "create", Object: "file"}}}}
	}
	return snap
}

// inputsOf derives ConclusionInputs from the same run snapshotOf builds, the
// way slice 8 will derive them from the record.
func inputsOf(outcomes []record.Outcome, changed, planned, dryRun, warned bool) ConclusionInputs {
	anySettledOK := false
	for _, outcome := range outcomes {
		switch EntityStateOf(record.PhaseSettled, outcome) {
		case Done, Skipped:
			anySettledOK = true
		}
	}
	return ConclusionInputs{
		Worst:        record.WorstOutcome(outcomes...),
		Changed:      changed,
		Planned:      planned,
		DryRun:       dryRun,
		AnySettledOK: anySettledOK,
		Warned:       warned && len(outcomes) > 0,
	}
}

// TestConclusionStateOfAgreesWithInferConclusion proves the derived headline
// is the one InferConclusion already computes, for a run whose Tasks all end
// as worst, over every combination of changed, planned, dryRun and warned.
func TestConclusionStateOfAgreesWithInferConclusion(t *testing.T) {
	for _, worst := range allOutcomes {
		for _, flags := range everyFlagCombination() {
			assertConclusionAgrees(t, []record.Outcome{worst}, flags)
		}
	}
}

// TestConclusionStateOfAgreesOnMixedRuns covers runs of two Tasks that end
// differently: every ordered pair of outcomes, over every flag combination.
func TestConclusionStateOfAgreesOnMixedRuns(t *testing.T) {
	for _, first := range allOutcomes {
		for _, second := range allOutcomes {
			for _, flags := range everyFlagCombination() {
				assertConclusionAgrees(t, []record.Outcome{first, second}, flags)
			}
		}
	}
}

func TestConclusionStateOfAgreesOnARunWithNoTasks(t *testing.T) {
	for _, flags := range everyFlagCombination() {
		assertConclusionAgrees(t, nil, flags)
	}
}

// TestConclusionStateOfReadsWarnedOnlyWhenNothingSettledOK pins the one
// headline the warned input decides: Warning needs a warning and no Done or
// Skipped Task, and a dry run does not turn it into Planned.
func TestConclusionStateOfReadsWarnedOnlyWhenNothingSettledOK(t *testing.T) {
	cases := []struct {
		name string
		in   ConclusionInputs
		want ConclusionState
	}{
		{"warned with nothing settled OK", ConclusionInputs{Warned: true}, StateWarning},
		{"warned dry run keeps the warning", ConclusionInputs{Warned: true, DryRun: true}, StateWarning},
		{"warned beside a settled OK Task", ConclusionInputs{Warned: true, AnySettledOK: true}, StateReady},
		{"warned but changed", ConclusionInputs{Warned: true, Changed: true}, StateChanged},
		{"warned but failed", ConclusionInputs{Warned: true, Worst: record.OutcomeFailed}, StateFailed},
	}
	for _, c := range cases {
		if got := ConclusionStateOf(c.in); got != c.want {
			t.Errorf("%s: ConclusionStateOf(%+v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

type runFlags struct{ changed, planned, dryRun, warned bool }

func everyFlagCombination() []runFlags {
	var all []runFlags
	for bits := range 16 {
		all = append(all, runFlags{bits&1 != 0, bits&2 != 0, bits&4 != 0, bits&8 != 0})
	}
	return all
}

func assertConclusionAgrees(t *testing.T, outcomes []record.Outcome, f runFlags) {
	t.Helper()
	want := InferConclusion(snapshotOf(outcomes, f.changed, f.planned, f.dryRun, f.warned)).State
	got := ConclusionStateOf(inputsOf(outcomes, f.changed, f.planned, f.dryRun, f.warned))
	if got != want {
		t.Errorf("outcomes=%v %+v: ConclusionStateOf = %q, InferConclusion = %q", outcomes, f, got, want)
	}
}
