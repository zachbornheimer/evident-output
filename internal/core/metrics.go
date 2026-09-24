package core

import (
	"slices"
	"time"
)

// RunMetrics is the run-level optimization aggregate (§39) Evo derives from
// every Task's TaskTiming and Resolution: where the run's time went and how
// much of its work was already satisfied. Derived, never recorded — callers
// read it from Conclusion.Metrics instead of summing Task rows themselves.
type RunMetrics struct {
	// Tasks counts every caller-declared Task, whatever its outcome.
	Tasks int
	// Executed, AlreadySatisfied, and NoWork count successful Tasks by
	// Resolution.
	Executed         int
	AlreadySatisfied int
	NoWork           int
	// DependencyWait, SchedulerWait, and Running sum the matching
	// TaskTiming spans across every Task.
	DependencyWait time.Duration
	SchedulerWait  time.Duration
	Running        time.Duration
	// PeakConcurrency is the most Tasks Running at one instant.
	PeakConcurrency int
}

// Metrics derives the run's RunMetrics from its Tasks, including those
// nested in Groups and Sequences. The synthetic Task that carries an
// output-level failure is not caller work and is excluded.
func (c Conclusion) Metrics() RunMetrics {
	var m RunMetrics
	var intervals []runningInterval
	walkTasks(c.Tasks, c.Collections, func(t TaskSnapshot) {
		if t.Synthetic() {
			return
		}
		m.count(t)
		if t.Timing.Running() > 0 {
			intervals = append(intervals, runningInterval{t.Timing.StartedAt, t.Timing.SettledAt})
		}
	})
	m.PeakConcurrency = peakOverlap(intervals)
	return m
}

func (m *RunMetrics) count(t TaskSnapshot) {
	m.Tasks++
	m.DependencyWait += t.Timing.DependencyWait()
	m.SchedulerWait += t.Timing.SchedulerWait()
	m.Running += t.Timing.Running()
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

// runningInterval is one Task's half-open [start, end) Running span.
type runningInterval struct{ start, end time.Time }

// peakOverlap is the most intervals open at one instant. Ends sort before
// starts at the same instant, so one Task settling as the next starts is
// back-to-back, not concurrent.
func peakOverlap(intervals []runningInterval) int {
	type edge struct {
		at    time.Time
		delta int
	}
	edges := make([]edge, 0, 2*len(intervals))
	for _, iv := range intervals {
		edges = append(edges, edge{iv.start, 1}, edge{iv.end, -1})
	}
	slices.SortFunc(edges, func(a, b edge) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return a.delta - b.delta
	})
	open, peak := 0, 0
	for _, e := range edges {
		open += e.delta
		peak = max(peak, open)
	}
	return peak
}
