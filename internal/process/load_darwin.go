package process

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// loadavgSysctl is the sysctl whose value is the kernel's struct loadavg:
// three fixed-point averages (uint32 each), padding, then their scale
// (long).
const loadavgSysctl = "vm.loadavg"

// loadavgScaleOffset is where struct loadavg's fscale starts: after three
// uint32 averages and the padding that aligns the long.
const loadavgScaleOffset = 16

// LoadAverage is the host's one-minute load average: how many threads were
// runnable, on average, over the last minute. ok is false where the host
// does not say.
func LoadAverage() (load float64, ok bool) {
	raw, err := unix.SysctlRaw(loadavgSysctl)
	if err != nil || len(raw) < loadavgScaleOffset+8 {
		return 0, false
	}
	scale := binary.NativeEndian.Uint64(raw[loadavgScaleOffset:])
	if scale == 0 {
		return 0, false
	}
	return float64(binary.NativeEndian.Uint32(raw)) / float64(scale), true
}
