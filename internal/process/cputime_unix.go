//go:build !windows

package process

import (
	"syscall"
	"time"
)

// CPUTime returns the CPU time this process has consumed so far (user plus
// system). Unlike wall time it does not count time the OS spent running
// something else.
func CPUTime() time.Duration {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		panic("process: getrusage: " + err.Error())
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}

// HasCPUTime reports whether CPUTime is a real process CPU clock here.
const HasCPUTime = true
