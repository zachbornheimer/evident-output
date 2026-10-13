// Package scaletest holds the timing estimator the wall-clock scaling tests
// share, so each one reads cost the same low-noise way.
package scaletest

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/clock"
)

// Samples is how many timed runs one estimate takes. Host load only ever
// adds wall time, so the fastest of several runs is the lowest-noise
// estimate of a size's cost; a single sample is noise-dominated.
const Samples = 5

// CheapSamples is for runs of a few milliseconds, where more samples cost
// little and a single timer hiccup is a large share of the measurement.
const CheapSamples = 15

// Fastest calls timed samples times and returns the smallest duration it
// reported. timed runs the work once and returns what that run cost.
func Fastest(samples int, timed func() time.Duration) time.Duration {
	best := timed()
	for range samples - 1 {
		best = min(best, timed())
	}
	return best
}

// Elapsed returns how long run took.
func Elapsed(run func()) time.Duration {
	wall := clock.System()
	start := wall.Now()
	run()
	return wall.Since(start)
}
