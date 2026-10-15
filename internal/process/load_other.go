//go:build !darwin && !linux

package process

// LoadAverage is the host's one-minute load average; this platform does not
// say, so ok is always false.
func LoadAverage() (load float64, ok bool) { return 0, false }
