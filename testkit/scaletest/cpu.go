package scaletest

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// CPUElapsed returns the process CPU time run consumed. Unlike Elapsed it
// does not count time the OS spent running something else: a long run is
// preempted more often than a short one, so under host load wall time
// inflates the larger size of a scaling test and fakes superlinear cost.
// Where the platform has no process CPU clock (Windows) it is Elapsed.
func CPUElapsed(run func()) time.Duration {
	if !process.HasCPUTime {
		return Elapsed(run)
	}
	before := process.CPUTime()
	run()
	return process.CPUTime() - before
}
