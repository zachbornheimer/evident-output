package process

import (
	"os"
	"strconv"
	"strings"
)

// loadavgPath holds the host's load averages, the one-minute one first.
const loadavgPath = "/proc/loadavg"

// LoadAverage is the host's one-minute load average: how many threads were
// runnable, on average, over the last minute. ok is false where the host
// does not say.
func LoadAverage() (load float64, ok bool) {
	raw, err := os.ReadFile(loadavgPath)
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0, false
	}
	load, err = strconv.ParseFloat(fields[0], 64)
	return load, err == nil
}
