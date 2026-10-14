package scaletest

import (
	"fmt"
	"runtime"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// HostIsBusy reports whether the host is too loaded for a wall-clock bound
// to mean anything: its one-minute load average exceeds the cores it has.
// A host that does not report its load counts as idle. reason says why, for
// the caller's skip message.
func HostIsBusy() (busy bool, reason string) {
	load, ok := process.LoadAverage()
	cores := runtime.NumCPU()
	if !ok || load <= float64(cores) {
		return false, ""
	}
	return true, fmt.Sprintf("host load average %.1f exceeds its %d cores", load, cores)
}
