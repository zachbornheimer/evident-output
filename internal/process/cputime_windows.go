//go:build windows

package process

import "time"

// CPUTime is zero here: Windows has no portable process CPU clock in the
// standard library. Check HasCPUTime before relying on it.
func CPUTime() time.Duration { return 0 }

// HasCPUTime reports whether CPUTime is a real process CPU clock here.
const HasCPUTime = false
