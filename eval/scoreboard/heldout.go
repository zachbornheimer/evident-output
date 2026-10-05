package scoreboard

import (
	"fmt"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/eval/runner"
)

const heldOutSubpath = ".evo-eval/heldout"

// HeldOutDir is where held-out tasks live, outside the repository.
func HeldOutDir(home string) string { return filepath.Join(home, filepath.FromSlash(heldOutSubpath)) }

// Aggregate is all a held-out run may reveal: counts and rates. It holds no
// task id, prompt, or transcript, so printing it cannot leak the tasks.
type Aggregate struct {
	Tasks         int
	Samples       int
	Passes        int
	PassAt1       float64
	PassPowerKAll float64 // fraction of tasks where every sample passed
}

// HeldOut reduces records to an Aggregate.
func HeldOut(records []runner.Record) Aggregate {
	stats := Compute(records)
	agg := Aggregate{Tasks: len(stats)}
	powerK := 0
	for _, task := range stats {
		agg.Samples += task.Samples
		agg.Passes += task.Passes
		if task.PassPowerK {
			powerK++
		}
	}
	if agg.Samples > 0 {
		agg.PassAt1 = float64(agg.Passes) / float64(agg.Samples)
	}
	if agg.Tasks > 0 {
		agg.PassPowerKAll = float64(powerK) / float64(agg.Tasks)
	}
	return agg
}

// String is the one-line report.
func (a Aggregate) String() string {
	return fmt.Sprintf("held-out: tasks=%d samples=%d pass@1=%.3f pass^k=%.3f", a.Tasks, a.Samples, a.PassAt1, a.PassPowerKAll)
}
