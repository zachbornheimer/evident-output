package core

import (
	"testing"
	"time"
)

var metricsEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// at is metricsEpoch plus n seconds — one readable timeline unit per test row.
func at(n int) time.Time { return metricsEpoch.Add(time.Duration(n) * time.Second) }

func secs(n int) time.Duration { return time.Duration(n) * time.Second }

// ran is a Task that submitted work at submitted, became eligible at
// eligible, started at started, and settled at settled.
func ran(submitted, eligible, started, settled int) TaskTiming {
	return TaskTiming{DeclaredAt: at(0), SubmittedAt: at(submitted), EligibleAt: at(eligible), StartedAt: at(started), SettledAt: at(settled)}
}

func TestTaskTiming_SpansBetweenLifecycleBoundaries(t *testing.T) {
	timing := ran(1, 2, 5, 9)
	checks := []struct {
		name      string
		got, want time.Duration
	}{
		{"AwaitingDefinition", timing.AwaitingDefinition(), secs(1)},
		{"DependencyWait", timing.DependencyWait(), secs(1)},
		{"SchedulerWait", timing.SchedulerWait(), secs(3)},
		{"Queued", timing.Queued(), secs(4)},
		{"Running", timing.Running(), secs(4)},
		{"Total", timing.Total(), secs(9)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if sum := timing.AwaitingDefinition() + timing.Queued() + timing.Running(); sum != timing.Total() {
		t.Errorf("AwaitingDefinition+Queued+Running = %v, want Total %v", sum, timing.Total())
	}
}

func TestTaskTiming_UncrossedBoundaryYieldsZeroNotANegativeOrEpochSpan(t *testing.T) {
	neverStarted := TaskTiming{DeclaredAt: at(0), SubmittedAt: at(1), SettledAt: at(4)}
	if got := neverStarted.Running(); got != 0 {
		t.Fatalf("Running of a never-started task = %v, want 0", got)
	}
	if got := neverStarted.Queued(); got != 0 {
		t.Fatalf("Queued of a never-started task = %v, want 0", got)
	}
	if got := neverStarted.Total(); got != secs(4) {
		t.Fatalf("Total = %v, want 4s", got)
	}
	// A Task resolved on the caller's stack never submits work.
	noWork := TaskTiming{DeclaredAt: at(0), StartedAt: at(1), SettledAt: at(3)}
	if noWork.Queued() != 0 || noWork.AwaitingDefinition() != 0 || noWork.Running() != secs(2) {
		t.Fatalf("no-work spans = (queued %v, awaiting %v, running %v), want (0, 0, 2s)", noWork.Queued(), noWork.AwaitingDefinition(), noWork.Running())
	}
}

func TestTaskTiming_QueuedIsAlwaysTheSumOfItsWaits(t *testing.T) {
	for _, timing := range []TaskTiming{
		ran(0, 3, 3, 5),
		ran(2, 2, 4, 5),
		{DeclaredAt: at(0), SubmittedAt: at(1), SettledAt: at(3)},
		{},
	} {
		if timing.Queued() != timing.DependencyWait()+timing.SchedulerWait() {
			t.Errorf("%+v: Queued %v != DependencyWait %v + SchedulerWait %v", timing, timing.Queued(), timing.DependencyWait(), timing.SchedulerWait())
		}
	}
}

func doneTask(res Resolution, timing TaskTiming) TaskSnapshot {
	return TaskSnapshot{State: Done, Resolution: res, Timing: timing}
}

func withID(id string, t TaskSnapshot) TaskSnapshot {
	t.ID = id
	return t
}

func TestConclusionMetrics_DerivesResolutionCountsWaitsAndPeakConcurrency(t *testing.T) {
	c := Conclusion{
		Tasks: []TaskSnapshot{
			withID("a", doneTask(ResolutionExecuted, ran(0, 0, 0, 4))),
		},
		Collections: []TasksSnapshot{{
			Tasks: []TaskSnapshot{
				withID("b", doneTask(ResolutionAlreadySatisfied, ran(0, 0, 1, 2))),
				{ID: "c", State: Failed, Resolution: ResolutionNoWork, Timing: ran(0, 0, 2, 3)},
			},
			Collections: []TasksSnapshot{{
				Tasks: []TaskSnapshot{
					withID("d", doneTask(ResolutionNoWork, ran(0, 4, 4, 6))),
				},
			}},
		}},
	}
	want := RunMetrics{
		Tasks:            4,
		Executed:         1,
		AlreadySatisfied: 1,
		NoWork:           1,
		Defined:          4,
		DependencyWait:   secs(4),
		SchedulerWait:    secs(3),
		Running:          secs(8),
		CriticalPath:     secs(4),
		PeakConcurrency:  2,
	}
	if got := c.Metrics(); got != want {
		t.Fatalf("Metrics() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestConclusionMetrics_BackToBackTasksDoNotOverlap(t *testing.T) {
	c := Conclusion{Tasks: []TaskSnapshot{
		doneTask(ResolutionExecuted, ran(0, 0, 0, 1)),
		doneTask(ResolutionExecuted, ran(0, 0, 1, 2)),
	}}
	if got := c.Metrics().PeakConcurrency; got != 1 {
		t.Fatalf("PeakConcurrency = %d, want 1: one task settling as the next starts is not overlap", got)
	}
}

func TestConclusionMetrics_ExcludesSyntheticOutcomeCarrier(t *testing.T) {
	synthetic := NewTaskSnapshot(TaskSnapshot{State: Failed}, TaskInternals{Synthetic: true})
	c := Conclusion{Tasks: []TaskSnapshot{synthetic}}
	if got := c.Metrics(); got != (RunMetrics{}) {
		t.Fatalf("Metrics() = %+v, want zero: the synthetic task is not caller work", got)
	}
}

func TestConclusionMetrics_SumsPhaseTimesOperationsAndEvidence(t *testing.T) {
	phase := func(n int) PhaseTime { return PhaseTime{Entries: 1, Duration: secs(n)} }
	entered := doneTask(ResolutionExecuted, ran(0, 0, 0, 5))
	entered.Timing.Definition, entered.Timing.Provenance, entered.Timing.TrackedState = phase(4), phase(1), phase(2)
	entered.Evidence.Before = EvidencePhase{Evaluated: true}
	entered.Timing.Evidence = PhaseTime{Entries: 2, Duration: secs(1)}
	entered.Operations = OperationCounts{Current: 1, Executed: 3, BasisDrift: 1, Changed: 1, Unchanged: 2}
	skipped := doneTask(ResolutionAlreadySatisfied, ran(0, 0, 0, 1))
	skipped.Evidence.Before = EvidencePhase{Evaluated: true, Satisfied: true}
	skipped.Timing.Evidence = phase(1)
	skipped.Operations = OperationCounts{Current: 2}

	m := Conclusion{Tasks: []TaskSnapshot{withID("entered", entered), withID("skipped", skipped)}}.Metrics()
	durations := map[string][2]time.Duration{
		"Definition":   {m.Definition, secs(4)},
		"Evidence":     {m.Evidence, secs(2)},
		"Provenance":   {m.Provenance, secs(1)},
		"TrackedState": {m.TrackedState, secs(2)},
	}
	for name, d := range durations {
		if d[0] != d[1] {
			t.Errorf("%s = %v, want %v", name, d[0], d[1])
		}
	}
	rates := map[string][2]float64{
		"CallbackEntryRate":      {m.CallbackEntryRate(), 0.5},
		"VerifySatisfiedRate":    {m.VerifySatisfiedRate(), 0.5},
		"HitRate":                {m.Operations.HitRate(), 0.5},
		"BasisInvalidationRate":  {m.Operations.BasisInvalidationRate(), 1.0 / 6},
		"ChangeRate":             {m.Operations.ChangeRate(), 1.0 / 3},
		"PropagationStoppedRate": {m.Operations.PropagationStoppedRate(), 2.0 / 3},
	}
	for name, r := range rates {
		if r[0] != r[1] {
			t.Errorf("%s = %v, want %v", name, r[0], r[1])
		}
	}
}

func TestRates_ZeroWholeIsZeroNotNaN(t *testing.T) {
	var m RunMetrics
	if m.CallbackEntryRate() != 0 || m.VerifySatisfiedRate() != 0 || m.Operations.HitRate() != 0 || m.Operations.PropagationStoppedRate() != 0 {
		t.Fatalf("rates over nothing must be 0, got %+v", m)
	}
}

// taskAfter is a Task that ran for running seconds, declared After preds.
func taskAfter(id string, running int, preds ...string) TaskSnapshot {
	return NewTaskSnapshot(withID(id, doneTask(ResolutionExecuted, ran(0, 0, 0, running))), TaskInternals{After: preds})
}

func TestConclusionMetrics_CriticalPathFollowsAfterAndSequenceEdges(t *testing.T) {
	c := Conclusion{
		Tasks: []TaskSnapshot{
			taskAfter("fetch", 2, "categories"),
			taskAfter("report", 1, "fetch", "lint"),
			taskAfter("lint", 1),
		},
		Collections: []TasksSnapshot{{
			ID: "categories",
			Tasks: []TaskSnapshot{
				taskAfter("branches", 3),
				taskAfter("worktrees", 1),
			},
			Collections: []TasksSnapshot{{
				ID:         "steps",
				Sequential: true,
				Tasks:      []TaskSnapshot{taskAfter("plan", 2), taskAfter("apply", 3)},
			}},
		}},
	}
	// plan(2) → apply(3) → fetch(2) → report(1) outweighs branches(3).
	if got := c.Metrics().CriticalPath; got != secs(8) {
		t.Fatalf("CriticalPath = %v, want 8s", got)
	}
}

func TestConclusionMetrics_CriticalPathSurvivesADependencyCycle(t *testing.T) {
	c := Conclusion{Tasks: []TaskSnapshot{taskAfter("a", 1, "b"), taskAfter("b", 2, "a")}}
	if got := c.Metrics().CriticalPath; got != secs(3) {
		t.Fatalf("CriticalPath = %v, want 3s (the cycle stops at the repeat)", got)
	}
}
