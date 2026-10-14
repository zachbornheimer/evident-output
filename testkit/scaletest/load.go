package scaletest

import (
	"fmt"
	"runtime"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// HostIsBusy reports whether the host is too loaded for a wall-clock bound
// to mean anything: its one-minute load average exceeds the cores it has.
// A host that does not report its load counts as idle. reason says why, for
// the caller's skip message.
func HostIsBusy() (busy bool, reason string) {
	cores := runtime.NumCPU()
	busy, load := loadExceeds(float64(cores))
	if !busy {
		return false, ""
	}
	return true, fmt.Sprintf("host load average %.1f exceeds its %d cores", load, cores)
}

// HostLoadExceeds reports whether the host's one-minute load average is
// above limit, for a test whose bound was recorded at a known load. A host
// that does not report its load counts as idle. reason says why, for the
// caller's skip message.
func HostLoadExceeds(limit float64) (busy bool, reason string) {
	busy, load := loadExceeds(limit)
	if !busy {
		return false, ""
	}
	return true, fmt.Sprintf("host load average %.1f exceeds %.0f", load, limit)
}

// loadExceeds reports whether the load average is above limit, and the load.
func loadExceeds(limit float64) (exceeds bool, load float64) {
	load, ok := process.LoadAverage()
	return ok && load > limit, load
}
