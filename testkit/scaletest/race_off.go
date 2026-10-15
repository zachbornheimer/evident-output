//go:build !race

package scaletest

// RaceDetector reports whether this binary was built with -race. The
// detector instruments every allocation, so bytes-per-op bounds measured
// without it do not hold.
const RaceDetector = false
