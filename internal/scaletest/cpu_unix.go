//go:build !windows

package scaletest

import (
	"syscall"
	"time"
)

// CPUElapsed returns the process CPU time run consumed. Unlike Elapsed it
// does not count time the OS spent running something else: a long run is
// preempted more often than a short one, so under host load wall time
// inflates the larger size of a scaling test and fakes superlinear cost.
func CPUElapsed(run func()) time.Duration {
	before := processCPU()
	run()
	return processCPU() - before
}

func processCPU() time.Duration {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		panic("scaletest: getrusage: " + err.Error())
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}
