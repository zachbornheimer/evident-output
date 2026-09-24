package core

import (
	"testing"
	"time"
)

var metricsEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// at is metricsEpoch plus n seconds — one readable timeline unit per test row.
func at(n int) time.Time { return metricsEpoch.Add(time.Duration(n) * time.Second) }

func TestTaskTiming_SpansBetweenLifecycleBoundaries(t *testing.T) {
	timing := TaskTiming{DeclaredAt: at(0), EligibleAt: at(2), StartedAt: at(5), SettledAt: at(9)}
	checks := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"DependencyWait", timing.DependencyWait(), 2 * time.Second},
		{"SchedulerWait", timing.SchedulerWait(), 3 * time.Second},
		{"Queued", timing.Queued(), 5 * time.Second},
		{"Running", timing.Running(), 4 * time.Second},
		{"Total", timing.Total(), 9 * time.Second},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestTaskTiming_UncrossedBoundaryYieldsZeroNotANegativeOrEpochSpan(t *testing.T) {
	neverStarted := TaskTiming{DeclaredAt: at(0), SettledAt: at(4)}
	if got := neverStarted.Running(); got != 0 {
		t.Fatalf("Running of a never-started task = %v, want 0", got)
	}
	if got := neverStarted.Queued(); got != 0 {
		t.Fatalf("Queued of a never-started task = %v, want 0", got)
	}
	if got := neverStarted.Total(); got != 4*time.Second {
		t.Fatalf("Total = %v, want 4s", got)
	}
	// Doing before Define promotes Running ahead of eligibility: the
	// scheduler never made it wait, so the wait is zero, never negative.
	startedFirst := TaskTiming{DeclaredAt: at(0), StartedAt: at(1), EligibleAt: at(3)}
	if got := startedFirst.SchedulerWait(); got != 0 {
		t.Fatalf("SchedulerWait with StartedAt before EligibleAt = %v, want 0", got)
	}
}

func doneTask(res Resolution, timing TaskTiming) TaskSnapshot {
	return TaskSnapshot{State: Done, Resolution: res, Timing: timing}
}

func TestConclusionMetrics_DerivesResolutionCountsWaitsAndPeakConcurrency(t *testing.T) {
	c := Conclusion{
		Tasks: []TaskSnapshot{
			doneTask(ResolutionExecuted, TaskTiming{DeclaredAt: at(0), EligibleAt: at(0), StartedAt: at(0), SettledAt: at(4)}),
		},
		Collections: []TasksSnapshot{{
			Tasks: []TaskSnapshot{
				doneTask(ResolutionAlreadySatisfied, TaskTiming{DeclaredAt: at(0), EligibleAt: at(0), StartedAt: at(1), SettledAt: at(2)}),
				{State: Failed, Resolution: ResolutionNoWork, Timing: TaskTiming{DeclaredAt: at(0), EligibleAt: at(0), StartedAt: at(2), SettledAt: at(3)}},
			},
			Collections: []TasksSnapshot{{
				Tasks: []TaskSnapshot{
					doneTask(ResolutionNoWork, TaskTiming{DeclaredAt: at(0), EligibleAt: at(4), StartedAt: at(4), SettledAt: at(6)}),
				},
			}},
		}},
	}
	want := RunMetrics{
		Tasks:            4,
		Executed:         1,
		AlreadySatisfied: 1,
		NoWork:           1,
		DependencyWait:   4 * time.Second,
		SchedulerWait:    3 * time.Second,
		Running:          8 * time.Second,
		PeakConcurrency:  2,
	}
	if got := c.Metrics(); got != want {
		t.Fatalf("Metrics() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestConclusionMetrics_BackToBackTasksDoNotOverlap(t *testing.T) {
	c := Conclusion{Tasks: []TaskSnapshot{
		doneTask(ResolutionExecuted, TaskTiming{DeclaredAt: at(0), EligibleAt: at(0), StartedAt: at(0), SettledAt: at(1)}),
		doneTask(ResolutionExecuted, TaskTiming{DeclaredAt: at(0), EligibleAt: at(0), StartedAt: at(1), SettledAt: at(2)}),
	}}
	if got := c.Metrics().PeakConcurrency; got != 1 {
		t.Fatalf("PeakConcurrency = %d, want 1: one task settling as the next starts is not overlap", got)
	}
}

func TestConclusionMetrics_ExcludesSyntheticOutcomeCarrier(t *testing.T) {
	synthetic := NewTaskSnapshot(TaskSnapshot{State: Failed}, time.Time{}, true, false)
	c := Conclusion{Tasks: []TaskSnapshot{synthetic}}
	if got := c.Metrics(); got != (RunMetrics{}) {
		t.Fatalf("Metrics() = %+v, want zero: the synthetic task is not caller work", got)
	}
}
