package core

import "time"

// RunMetrics is the run-level optimization aggregate (§39) Evo derives from
// every Task's TaskTiming, OperationCounts, Evidence, and Resolution.
// Derived, never recorded — callers read it from Conclusion.Metrics
// instead of summing Task rows themselves.
//
// It explains where the run's time went: waiting on dependencies
// (DependencyWait), waiting on scheduler capacity (SchedulerWait),
// executing work (Running), revalidating uncertain definitions
// (Definition), checking provenance (Provenance), and verifying tracked
// state (Evidence and TrackedState). Provenance and TrackedState are spent
// inside Define callbacks, so they are also part of Definition.
type RunMetrics struct {
	// Tasks counts every caller-declared Task, whatever its outcome.
	Tasks int
	// Executed, AlreadySatisfied, and NoWork count successful Tasks by
	// Resolution.
	Executed         int
	AlreadySatisfied int
	NoWork           int
	// Defined counts Tasks that submitted a Define callback; Entered counts
	// the callbacks Evo actually entered because no current proof existed.
	Defined int
	Entered int
	// Verified counts Tasks whose Verify was evaluated before their
	// callback; VerifiedCurrent counts those Verify proved already
	// satisfied, so the callback was skipped.
	Verified        int
	VerifiedCurrent int

	// DependencyWait, SchedulerWait, and Running sum the matching
	// TaskTiming spans across every Task.
	DependencyWait time.Duration
	SchedulerWait  time.Duration
	Running        time.Duration
	// Definition, Evidence, Provenance, and TrackedState sum the matching
	// TaskTiming phase times across every Task.
	Definition   time.Duration
	Evidence     time.Duration
	Provenance   time.Duration
	TrackedState time.Duration
	// CriticalPath is the longest chain of Running time through the run's
	// dependency edges (After, and each Sequence step on the one before
	// it): the shortest the run could take with unlimited capacity.
	CriticalPath time.Duration
	// PeakConcurrency is the most Tasks whose Running spans overlapped at
	// one instant. It counts every running Task: work the scheduler
	// started, work an inline Wait ran on the caller's stack, and Tasks
	// resolved without submitting work. MaxConcurrency bounds only the
	// scheduler's own starts, so PeakConcurrency can exceed it.
	PeakConcurrency int

	// Operations sums every Task's tracked-operation tallies.
	Operations OperationCounts
}

// CallbackEntryRate is the share of Define callbacks Evo entered: the
// opaque-callback entry rate. The rest were proven current and skipped.
func (m RunMetrics) CallbackEntryRate() float64 { return ratio(m.Entered, m.Defined) }

// VerifySatisfiedRate is the share of evaluated Verify checks that proved
// their Task already satisfied: the Verify-driven already-satisfied rate.
func (m RunMetrics) VerifySatisfiedRate() float64 { return ratio(m.VerifiedCurrent, m.Verified) }

// Metrics derives the run's RunMetrics from its Tasks, including those
// nested in Groups and Sequences. The synthetic Task that carries an
// output-level failure is not caller work and is excluded.
func (c Conclusion) Metrics() RunMetrics {
	var m RunMetrics
	var running runningIntervals
	walkTasks(c.Tasks, c.Collections, func(t TaskSnapshot) {
		if t.Synthetic() {
			return
		}
		m.count(t)
		running.add(t.Timing)
	})
	m.PeakConcurrency = running.peak()
	m.CriticalPath = newDependencyGraph(c).criticalPath()
	return m
}

func (m *RunMetrics) count(t TaskSnapshot) {
	m.Tasks++
	m.countSpans(t.Timing)
	m.Operations = m.Operations.plus(t.Operations)
	if !t.Timing.SubmittedAt.IsZero() {
		m.Defined++
	}
	if t.Timing.Definition.Entries > 0 {
		m.Entered++
	}
	if t.Evidence.Before.Evaluated {
		m.Verified++
		if t.Evidence.Before.Satisfied {
			m.VerifiedCurrent++
		}
	}
	m.countResolution(t)
}

func (m *RunMetrics) countSpans(timing TaskTiming) {
	m.DependencyWait += timing.DependencyWait()
	m.SchedulerWait += timing.SchedulerWait()
	m.Running += timing.Running()
	m.Definition += timing.Definition.Duration
	m.Evidence += timing.Evidence.Duration
	m.Provenance += timing.Provenance.Duration
	m.TrackedState += timing.TrackedState.Duration
}

func (m *RunMetrics) countResolution(t TaskSnapshot) {
	if t.State != Done {
		return
	}
	switch t.Resolution {
	case ResolutionExecuted:
		m.Executed++
	case ResolutionAlreadySatisfied:
		m.AlreadySatisfied++
	case ResolutionNoWork:
		m.NoWork++
	}
}

func walkTasks(tasks []TaskSnapshot, collections []TasksSnapshot, visit func(TaskSnapshot)) {
	for _, t := range tasks {
		visit(t)
	}
	for _, col := range collections {
		walkTasks(col.Tasks, col.Collections, visit)
	}
}
