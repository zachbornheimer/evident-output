// Elapsed: the one compact duration form (contract §32).
package text

import (
	"fmt"
	"time"
)

const (
	secondsPerMinute = 60
	minutesPerHour   = 60
	hoursPerDay      = 24
)

// FormatElapsed renders d in the single compact form every elapsed and quiet
// duration shares: "2s" under a minute, "4m12s" under an hour, "3h04m" under
// a day, "2d3h" beyond. Sub-second precision is dropped (floored), and zero
// or negative durations clamp to "0s". The output always carries units and
// never takes the Go Duration.String shape.
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int64(d / time.Second)
	seconds := totalSeconds % secondsPerMinute
	totalMinutes := totalSeconds / secondsPerMinute
	totalHours := totalMinutes / minutesPerHour
	days := totalHours / hoursPerDay
	switch {
	case totalMinutes == 0:
		return fmt.Sprintf("%ds", seconds)
	case totalHours == 0:
		return fmt.Sprintf("%dm%ds", totalMinutes, seconds)
	case days == 0:
		return fmt.Sprintf("%dh%02dm", totalHours, totalMinutes%minutesPerHour)
	default:
		return fmt.Sprintf("%dd%dh", days, totalHours%hoursPerDay)
	}
}
