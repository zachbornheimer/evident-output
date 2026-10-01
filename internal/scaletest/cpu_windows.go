//go:build windows

package scaletest

import "time"

// CPUElapsed is Elapsed here: Windows has no portable process CPU clock
// in the standard library.
func CPUElapsed(run func()) time.Duration { return Elapsed(run) }
