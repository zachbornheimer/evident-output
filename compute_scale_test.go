package evo

import (
	"context"
	"fmt"
	"io"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/testkit/scaletest"
)

const (
	zqShapePackagesPerManager = 2000
	zqShapeManagers           = 2
	zqShapeStepsPerTask       = 70
	zqShapeModifiedTasks      = 20
	genericBuilderTasks       = 100_000
	scaleMaxEntities          = 500_000

	// scaleSmallDivisor sets the small run at 1/10 of the full task count.
	scaleSmallDivisor = 10
	// maxScaleRatio bounds cpu(full)/cpu(small), where cpu is the process
	// CPU time a run consumed. Linear growth for scaleSmallDivisor times
	// the Tasks gives 10; 2x headroom absorbs cache and GC effects of the
	// larger heap and still fails any quadratic behavior (which gives
	// about 100).
	maxScaleRatio = 20.0
	// maxBytesPerTask bounds cumulative allocation per Task (declaring,
	// scheduling, and running it), so a per-Task memory regression fails
	// regardless of host load.
	maxBytesPerTask = 128 << 10
	// maxBytesRatio bounds bytesPerTask(full)/bytesPerTask(small). Linear
	// growth keeps it near 1 (map and slice growth account for the rest);
	// 2x still fails a builder whose per-Task allocation grows with the
	// Task count.
	maxBytesRatio = 2.0
)

func newScaleOutput(t *testing.T) *Run {
	t.Helper()
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard}}, tuning{maxEntities: scaleMaxEntities})
	t.Cleanup(func() { _ = out.close() })
	return out
}

// scaleSample is what one run of n Tasks cost.
type scaleSample struct {
	cpu          time.Duration
	bytesPerTask float64
}

// sampleRun measures the CPU time run consumed and the bytes it allocated
// per Task. CPU time does not count the time the OS spent running another
// process, and allocated bytes do not depend on scheduling at all; a hung
// run is stopped by go test's own timeout.
func sampleRun(tasks int, run func()) scaleSample {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	cpu := scaletest.CPUElapsed(run)
	runtime.ReadMemStats(&after)
	return scaleSample{cpu: cpu, bytesPerTask: float64(after.TotalAlloc-before.TotalAlloc) / float64(tasks)}
}

// scalePair is one back-to-back small and full run. Pairing them puts both
// under the same host load, so a pair's ratios reflect the code more than
// the moment.
type scalePair struct{ small, full scaleSample }

func (p scalePair) cpuRatio() float64 { return float64(p.full.cpu) / float64(p.small.cpu) }

func (p scalePair) bytesRatio() float64 { return p.full.bytesPerTask / p.small.bytesPerTask }

// assertScalesLinearly runs the shape at 1/10 and at full size in pairs and
// checks cost grows near-linearly in CPU time and memory. Load and cache
// contention only ever inflate a pair's CPU ratio, while a superlinear
// builder inflates every pair to about the square (100x), so one pair within
// the bound proves linear growth; it stops there, and fails only when none of
// scaletest.Samples pairs fits. Allocated bytes are load-independent, so the
// per-Task memory bounds hold for every pair.
func assertScalesLinearly(t *testing.T, name string, fullTasks int, run func(tasks int)) {
	t.Helper()
	smallTasks := fullTasks / scaleSmallDivisor
	run(smallTasks) // warm-up: first-run costs are not scaling
	best := scalePair{}
	bestBytesRatio := math.Inf(1)
	for pairs := range scaletest.Samples {
		pair := scalePair{
			small: sampleRun(smallTasks, func() { run(smallTasks) }),
			full:  sampleRun(fullTasks, func() { run(fullTasks) }),
		}
		if pair.full.bytesPerTask > maxBytesPerTask {
			t.Fatalf("%s: %.0f bytes allocated per Task, want <= %d", name, pair.full.bytesPerTask, maxBytesPerTask)
		}
		bestBytesRatio = min(bestBytesRatio, pair.bytesRatio())
		if pairs == 0 || pair.cpuRatio() < best.cpuRatio() {
			best = pair
		}
		if best.cpuRatio() <= maxScaleRatio {
			break
		}
	}
	t.Logf("%s: small %s CPU, full %s CPU (ratio %.1f, limit %.0f); %.0f bytes/Task (ratio %.2f, limit %.0f)",
		name, best.small.cpu, best.full.cpu, best.cpuRatio(), maxScaleRatio, best.full.bytesPerTask, bestBytesRatio, maxBytesRatio)
	if best.cpuRatio() > maxScaleRatio {
		t.Fatalf("%s: %dx the Tasks cost %.1fx the CPU time in every pair, want <= %.0fx", name, scaleSmallDivisor, best.cpuRatio(), maxScaleRatio)
	}
	if bestBytesRatio > maxBytesRatio {
		t.Fatalf("%s: bytes per Task grew %.2fx for %dx the Tasks, want <= %.0fx", name, bestBytesRatio, scaleSmallDivisor, maxBytesRatio)
	}
}

func runBulkBuilder(t *testing.T, tasks int) {
	t.Helper()
	out := newScaleOutput(t)
	out.Group("bulk").define(func(g *Group) {
		for i := range tasks {
			g.Task(fmt.Sprintf("task %d", i)).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestContainerDefine_HundredThousandTaskBuilderScalesLinearly(t *testing.T) {
	assertScalesLinearly(t, "bulk builder", genericBuilderTasks, func(tasks int) {
		runBulkBuilder(t, tasks)
	})
}
