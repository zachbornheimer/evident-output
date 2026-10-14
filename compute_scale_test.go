package evo

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/testkit/scaletest"
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
	// maxScaleRatio bounds elapsed(full)/elapsed(small). Linear growth for
	// scaleSmallDivisor times the Tasks gives 10; 2x headroom absorbs noise
	// and still fails any quadratic behavior (which gives about 100).
	maxScaleRatio = 20.0
	// maxBytesPerTask bounds cumulative allocation per Task (declaring,
	// scheduling, and running it), so a per-Task memory regression fails
	// regardless of host load.
	maxBytesPerTask = 128 << 10
	// hangGuard is not a performance budget. It only turns a hung run into
	// a failure with a message; host load cannot plausibly reach it.
	hangGuard = 10 * time.Minute
)

func newScaleOutput(t *testing.T) *Run {
	t.Helper()
	out := initRun(Config{stdout: io.Discard, plain: true, maxEntities: scaleMaxEntities})
	t.Cleanup(func() { _ = out.close() })
	return out
}

// scaleSample is what one run of n Tasks cost.
type scaleSample struct {
	elapsed      time.Duration
	bytesPerTask float64
}

// sampleRun times run and measures the bytes it allocated per Task.
func sampleRun(t *testing.T, tasks int, run func()) scaleSample {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	run()
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if elapsed > hangGuard {
		t.Fatalf("run of %d Tasks took %s: hung", tasks, elapsed)
	}
	return scaleSample{elapsed: elapsed, bytesPerTask: float64(after.TotalAlloc-before.TotalAlloc) / float64(tasks)}
}

// bestSample takes scaletest.Samples runs of tasks and returns the fastest
// elapsed time with the largest bytes per Task seen, so load cannot inflate
// the time and no run's allocation escapes the memory bound.
func bestSample(t *testing.T, tasks int, run func()) scaleSample {
	t.Helper()
	var worstBytes float64
	elapsed := scaletest.Fastest(scaletest.Samples, func() time.Duration {
		sample := sampleRun(t, tasks, run)
		worstBytes = max(worstBytes, sample.bytesPerTask)
		return sample.elapsed
	})
	return scaleSample{elapsed: elapsed, bytesPerTask: worstBytes}
}

// assertScalesLinearly runs the shape at 1/10 and at full size under the
// same load and checks cost grows near-linearly in time and memory. Both
// checks compare the process with itself, and each size is its fastest of
// scaletest.Samples runs, so host load cancels out.
func assertScalesLinearly(t *testing.T, name string, fullTasks int, run func(tasks int)) {
	t.Helper()
	smallTasks := fullTasks / scaleSmallDivisor
	run(smallTasks) // warm-up: first-run costs are not scaling
	small := bestSample(t, smallTasks, func() { run(smallTasks) })
	full := bestSample(t, fullTasks, func() { run(fullTasks) })
	ratio := float64(full.elapsed) / float64(small.elapsed)
	t.Logf("%s: small %s, full %s (ratio %.1f, limit %.0f); %.0f bytes/Task", name, small.elapsed, full.elapsed, ratio, maxScaleRatio, full.bytesPerTask)
	if ratio > maxScaleRatio {
		t.Fatalf("%s: %dx the Tasks cost %.1fx the time, want <= %.0fx", name, scaleSmallDivisor, ratio, maxScaleRatio)
	}
	if full.bytesPerTask > maxBytesPerTask {
		t.Fatalf("%s: %.0f bytes allocated per Task, want <= %d", name, full.bytesPerTask, maxBytesPerTask)
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
