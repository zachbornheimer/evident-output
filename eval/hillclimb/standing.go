// Package hillclimb is the accept/stop logic and the guards of the eval
// hill-climb loop: one worker edit per step to a tuned surface, kept only if
// the training tasks improve and nothing frozen was touched.
package hillclimb

import (
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/eval/runner"
)

const (
	// MinSampleGain is how many more samples must pass for an edit to count.
	MinSampleGain = 1
	// MaxTaskDrop is the per-task loss (in samples) that rejects an edit.
	MaxTaskDrop = 2
	// HeldOutPatience is how many consecutive held-out checks without gain
	// end the climb.
	HeldOutPatience = 2
)

// Standing is how the training tasks stand: passing samples per task out of
// SamplesPerTask each, plus the transcripts of the failing samples.
type Standing struct {
	SamplesPerTask int            `json:"samples_per_task"`
	Passes         map[string]int `json:"passes"`
	Failures       []Failure      `json:"-"`
	CostUSD        float64        `json:"cost_usd"`
}

// Failure is one failing sample handed to the worker as evidence.
type Failure struct {
	Task   string
	Sample int
	Record runner.Record
}

// StandingFromRecords tallies transcripts of a training run.
func StandingFromRecords(records []runner.Record) Standing {
	standing := Standing{Passes: map[string]int{}}
	perTask := map[string]int{}
	for _, record := range records {
		perTask[record.Task]++
		standing.CostUSD += record.CostUSD
		if _, seen := standing.Passes[record.Task]; !seen {
			standing.Passes[record.Task] = 0
		}
		if record.Passed {
			standing.Passes[record.Task]++
			continue
		}
		standing.Failures = append(standing.Failures, Failure{Task: record.Task, Sample: record.Sample, Record: record})
	}
	for _, count := range perTask {
		standing.SamplesPerTask = max(standing.SamplesPerTask, count)
	}
	return standing
}

// TotalPasses is the passing samples across all tasks.
func (s Standing) TotalPasses() int {
	total := 0
	for _, passes := range s.Passes {
		total += passes
	}
	return total
}

// Saturated is training pass^k at 100%: every sample of every task passed.
func (s Standing) Saturated() bool {
	if len(s.Passes) == 0 {
		return false
	}
	for _, passes := range s.Passes {
		if passes != s.SamplesPerTask {
			return false
		}
	}
	return true
}

// Decision is whether an edit is kept, and why.
type Decision struct {
	Accepted bool
	Reason   string
}

// Accept keeps an edit when training pass@1 rose by at least MinSampleGain
// samples and no task lost MaxTaskDrop or more samples.
func Accept(before, after Standing) Decision {
	tasks := make([]string, 0, len(before.Passes))
	for task := range before.Passes {
		tasks = append(tasks, task)
	}
	slices.Sort(tasks)
	for _, task := range tasks {
		if drop := before.Passes[task] - after.Passes[task]; drop >= MaxTaskDrop {
			return Decision{Reason: fmt.Sprintf("task %s dropped %d samples (limit %d)", task, drop, MaxTaskDrop-1)}
		}
	}
	gain := after.TotalPasses() - before.TotalPasses()
	if gain < MinSampleGain {
		return Decision{Reason: fmt.Sprintf("training passes rose by %d samples, need at least %d", gain, MinSampleGain)}
	}
	return Decision{Accepted: true, Reason: fmt.Sprintf("training passes rose by %d samples", gain)}
}
