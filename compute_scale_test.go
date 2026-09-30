package evo_test

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
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

func newScaleOutput(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, MaxEntities: scaleMaxEntities})
	t.Cleanup(func() { _ = out.Close() })
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

// assertScalesLinearly runs the shape at 1/10 and at full size under the
// same load and checks cost grows near-linearly in time and memory. Both
// checks compare the process with itself, so host load cancels out.
func assertScalesLinearly(t *testing.T, name string, fullTasks int, run func(tasks int)) {
	t.Helper()
	smallTasks := fullTasks / scaleSmallDivisor
	run(smallTasks) // warm-up: first-run costs are not scaling
	small := sampleRun(t, smallTasks, func() { run(smallTasks) })
	full := sampleRun(t, fullTasks, func() { run(fullTasks) })
	ratio := float64(full.elapsed) / float64(small.elapsed)
	t.Logf("%s: small %s, full %s (ratio %.1f, limit %.0f); %.0f bytes/Task", name, small.elapsed, full.elapsed, ratio, maxScaleRatio, full.bytesPerTask)
	if ratio > maxScaleRatio {
		t.Fatalf("%s: %dx the Tasks cost %.1fx the time, want <= %.0fx", name, scaleSmallDivisor, ratio, maxScaleRatio)
	}
	if full.bytesPerTask > maxBytesPerTask {
		t.Fatalf("%s: %.0f bytes allocated per Task, want <= %d", name, full.bytesPerTask, maxBytesPerTask)
	}
}

// runZqShape declares zq's real prune shape: two manager Groups under one
// builder, distinct-package Tasks that each report many progress steps,
// and a few Skipped "modified" Tasks.
func runZqShape(t *testing.T, packagesPerManager int) {
	t.Helper()
	out := newScaleOutput(t)
	packages := out.Sequence("consolidate packages")
	inventory := evo.Compute(packages.Task("discover installed packages"), func(context.Context) ([]string, error) {
		return []string{"npm", "composer"}, nil
	})
	centralize := packages.Group("centralize packages")
	centralize.Define(func(g *evo.GroupHandle) {
		for _, manager := range inventory.Get() {
			mg := g.Group(manager)
			for i := range packagesPerManager {
				task := mg.Task(fmt.Sprintf("centralize %s-pkg-%d", manager, i))
				task.Define(func(context.Context) error {
					for step := 1; step <= zqShapeStepsPerTask; step++ {
						task.Doing("step %d", step)
						task.Progress(step, zqShapeStepsPerTask)
					}
					return nil
				})
			}
		}
		kept := g.Group("modified checkouts")
		for i := range zqShapeModifiedTasks {
			kept.Task(fmt.Sprintf("keep modified pkg (checkout-%d)", i)).Skipped(evo.Reason("modified"))
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}

func TestContainerDefine_ZqShapedRunScalesLinearly(t *testing.T) {
	full := zqShapeManagers * zqShapePackagesPerManager
	assertScalesLinearly(t, "zq-shaped run", full, func(tasks int) {
		runZqShape(t, tasks/zqShapeManagers)
	})
}

func runBulkBuilder(t *testing.T, tasks int) {
	t.Helper()
	out := newScaleOutput(t)
	out.Group("bulk").Define(func(g *evo.GroupHandle) {
		for i := range tasks {
			g.Task(fmt.Sprintf("task %d", i)).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestContainerDefine_HundredThousandTaskBuilderScalesLinearly(t *testing.T) {
	assertScalesLinearly(t, "bulk builder", genericBuilderTasks, func(tasks int) {
		runBulkBuilder(t, tasks)
	})
}
