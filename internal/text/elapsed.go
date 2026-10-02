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
// a day, "2d3h" beyond. A smaller unit appears only when it is non-zero, so
// exact units read "1m", "1h", "1d". Sub-second precision is dropped
// (floored), and zero or negative durations clamp to "0s". The output always
// carries units and never takes the Go Duration.String shape.
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int64(d / time.Second)
	seconds := totalSeconds % secondsPerMinute
	totalMinutes := totalSeconds / secondsPerMinute
	minutes := totalMinutes % minutesPerHour
	totalHours := totalMinutes / minutesPerHour
	hours := totalHours % hoursPerDay
	days := totalHours / hoursPerDay
	switch {
	case totalMinutes == 0:
		return fmt.Sprintf("%ds", seconds)
	case totalHours == 0 && seconds == 0:
		return fmt.Sprintf("%dm", totalMinutes)
	case totalHours == 0:
		return fmt.Sprintf("%dm%02ds", totalMinutes, seconds)
	case days == 0 && minutes == 0:
		return fmt.Sprintf("%dh", totalHours)
	case days == 0:
		return fmt.Sprintf("%dh%02dm", totalHours, minutes)
	case hours == 0:
		return fmt.Sprintf("%dd", days)
	default:
		return fmt.Sprintf("%dd%dh", days, hours)
	}
}
