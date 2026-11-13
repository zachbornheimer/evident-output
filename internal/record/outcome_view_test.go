package record

import "testing"

var allOutcomes = []Outcome{
	OutcomeSucceeded, OutcomeSatisfied, OutcomeExcluded,
	OutcomeRefused, OutcomeFailed, OutcomeCancelled,
}

func TestEntityStateOfMapsEveryOutcomeToItsLegacyState(t *testing.T) {
	want := map[Outcome]EntityState{
		OutcomeSucceeded: Done,
		OutcomeSatisfied: Done,
		OutcomeExcluded:  Skipped,
		OutcomeRefused:   Blocked,
		OutcomeFailed:    Failed,
		OutcomeCancelled: Cancelled,
	}
	for _, outcome := range allOutcomes {
		if got := EntityStateOf(PhaseSettled, outcome); got != want[outcome] {
			t.Errorf("EntityStateOf(settled, %s) = %q, want %q", outcome, got, want[outcome])
		}
	}
}

func TestEntityStateOfIgnoresOutcomeBeforeSettling(t *testing.T) {
	for _, outcome := range allOutcomes {
		if got := EntityStateOf(PhasePending, outcome); got != Pending {
			t.Errorf("EntityStateOf(pending, %s) = %q, want %q", outcome, got, Pending)
		}
		if got := EntityStateOf(PhaseRunning, outcome); got != Running {
			t.Errorf("EntityStateOf(running, %s) = %q, want %q", outcome, got, Running)
		}
	}
}

func TestEntityStateOfRefusesAnUnknownOutcome(t *testing.T) {
	if got := EntityStateOf(PhaseSettled, "invented"); got != "" {
		t.Errorf("EntityStateOf(settled, invented) = %q, want the zero state", got)
	}
	if got := EntityStateOf("invented", OutcomeFailed); got != "" {
		t.Errorf("EntityStateOf(invented, failed) = %q, want the zero state", got)
	}
}

func TestResolutionOfKeepsTodaysThreeReasons(t *testing.T) {
	cases := []struct {
		outcome   Outcome
		defineRan bool
		want      Resolution
	}{
		{OutcomeSucceeded, true, ResolutionExecuted},
		{OutcomeSucceeded, false, ResolutionNoWork},
		{OutcomeSatisfied, false, ResolutionAlreadySatisfied},
		{OutcomeSatisfied, true, ResolutionAlreadySatisfied},
		{OutcomeExcluded, false, ResolutionNoWork},
		{OutcomeRefused, false, ResolutionNoWork},
		{OutcomeFailed, true, ResolutionNoWork},
		{OutcomeCancelled, false, ResolutionNoWork},
	}
	for _, c := range cases {
		if got := ResolutionOf(c.outcome, c.defineRan); got != c.want {
			t.Errorf("ResolutionOf(%s, defineRan=%t) = %q, want %q", c.outcome, c.defineRan, got, c.want)
		}
	}
}

// snapshotOf is a run whose Tasks settled to outcomes (one Task each), with
// a warning on the first Task when warned, plus the optional ledgers.
func snapshotOf(outcomes []Outcome, changed, planned, dryRun, warned bool) Snapshot {
	snap := Snapshot{DryRun: dryRun}
	for i, outcome := range outcomes {
		task := TaskSnapshot{State: EntityStateOf(PhaseSettled, outcome)}
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
// way slice 8 will derive them from the
func inputsOf(outcomes []Outcome, changed, planned, dryRun, warned bool) ConclusionInputs {
	anySettledOK := false
	for _, outcome := range outcomes {
		switch EntityStateOf(PhaseSettled, outcome) {
		case Done, Skipped:
			anySettledOK = true
		}
	}
	return ConclusionInputs{
		Worst:        WorstOutcome(outcomes...),
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
			assertConclusionAgrees(t, []Outcome{worst}, flags)
		}
	}
}

// TestConclusionStateOfAgreesOnMixedRuns covers runs of two Tasks that end
// differently: every ordered pair of outcomes, over every flag combination.
func TestConclusionStateOfAgreesOnMixedRuns(t *testing.T) {
	for _, first := range allOutcomes {
		for _, second := range allOutcomes {
			for _, flags := range everyFlagCombination() {
				assertConclusionAgrees(t, []Outcome{first, second}, flags)
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
		{"warned but failed", ConclusionInputs{Warned: true, Worst: OutcomeFailed}, StateFailed},
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

func assertConclusionAgrees(t *testing.T, outcomes []Outcome, f runFlags) {
	t.Helper()
	want := InferConclusion(snapshotOf(outcomes, f.changed, f.planned, f.dryRun, f.warned)).State
	got := ConclusionStateOf(inputsOf(outcomes, f.changed, f.planned, f.dryRun, f.warned))
	if got != want {
		t.Errorf("outcomes=%v %+v: ConclusionStateOf = %q, InferConclusion = %q", outcomes, f, got, want)
	}
}
