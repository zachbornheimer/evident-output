package engine

import "github.com/zachbornheimer/evident-output/internal/clock"

// The root package keeps compiling through these aliases until the root
// speaks internal/clock directly.
type (
	// TimeSource provides the current time for deterministic tests.
	TimeSource = clock.Clock
	// Scheduler is TimeSource's optional capability to run fn after a delay.
	Scheduler = clock.Scheduler
	// systemClock uses the real wall clock.
	systemClock = clock.Wall
	// fixedClock always returns the same instant.
	fixedClock = clock.Fixed
)

// wall is the real clock for waits that must stay real whatever Clock the
// application injected: spinner cadence, paint cost, throttled-write age.
var wall = clock.System()
