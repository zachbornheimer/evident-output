package wire

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// MetricsDoc is the run-level §39 optimization aggregate — the wire form of
// core.RunMetrics, carried as the final document's "data.metrics" and the
// JSONL run.finished payload's "metrics", so both machine projections state
// the same derived numbers.
type MetricsDoc struct {
	Tasks            int   `json:"tasks"`
	Executed         int   `json:"executed"`
	AlreadySatisfied int   `json:"already_satisfied"`
	NoWork           int   `json:"no_work"`
	DependencyWaitMs int64 `json:"dependency_wait_ms"`
	SchedulerWaitMs  int64 `json:"scheduler_wait_ms"`
	RunningMs        int64 `json:"running_ms"`
	PeakConcurrency  int   `json:"peak_concurrency"`
}

// ToMetricsDoc projects a Conclusion's derived RunMetrics onto the wire.
func ToMetricsDoc(c core.Conclusion) MetricsDoc {
	m := c.Metrics()
	return MetricsDoc{
		Tasks:            m.Tasks,
		Executed:         m.Executed,
		AlreadySatisfied: m.AlreadySatisfied,
		NoWork:           m.NoWork,
		DependencyWaitMs: m.DependencyWait.Milliseconds(),
		SchedulerWaitMs:  m.SchedulerWait.Milliseconds(),
		RunningMs:        m.Running.Milliseconds(),
		PeakConcurrency:  m.PeakConcurrency,
	}
}

// toTimingDoc projects one Task's lifecycle spans (spec §36 "timing").
func toTimingDoc(t core.TaskTiming) TimingDoc {
	return TimingDoc{
		QueuedMs:         ms(t.Queued()),
		RunningMs:        ms(t.Running()),
		TotalMs:          ms(t.Total()),
		DependencyWaitMs: ms(t.DependencyWait()),
		SchedulerWaitMs:  ms(t.SchedulerWait()),
	}
}

func ms(d time.Duration) int64 { return d.Milliseconds() }
