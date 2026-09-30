package evo_test

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	zqShapePackagesPerManager = 2000
	zqShapeStepsPerTask       = 70
	zqShapeModifiedTasks      = 20
	zqShapeBudget             = 60 * time.Second
	genericBuilderTasks       = 100_000
	genericBuilderBudget      = 60 * time.Second
	scaleMaxEntities          = 500_000
)

func newScaleOutput(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, MaxEntities: scaleMaxEntities})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

// TestContainerDefine_ZqShapedRunCompletesInBoundedTime declares zq's real
// prune shape: two manager Groups under one builder, thousands of
// distinct-package Tasks that each report many progress steps, and a few
// Skipped "modified" Tasks.
func TestContainerDefine_ZqShapedRunCompletesInBoundedTime(t *testing.T) {
	out := newScaleOutput(t)
	packages := out.Sequence("consolidate packages")
	inventory := evo.Compute(packages.Task("discover installed packages"), func(context.Context) ([]string, error) {
		return []string{"npm", "composer"}, nil
	})
	centralize := packages.Group("centralize packages")
	start := time.Now()
	centralize.Define(func(g *evo.GroupHandle) {
		for _, manager := range inventory.Get() {
			mg := g.Group(manager)
			for i := range zqShapePackagesPerManager {
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
	elapsed := time.Since(start)
	t.Logf("zq-shaped run: %d tasks, %d steps each, %s", 2*zqShapePackagesPerManager+zqShapeModifiedTasks, zqShapeStepsPerTask, elapsed)
	if elapsed > zqShapeBudget {
		t.Fatalf("took %s, budget %s", elapsed, zqShapeBudget)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}

func TestContainerDefine_HundredThousandTaskBuilderDeclaresInBoundedTime(t *testing.T) {
	out := newScaleOutput(t)
	start := time.Now()
	out.Group("bulk").Define(func(g *evo.GroupHandle) {
		for i := range genericBuilderTasks {
			g.Task(fmt.Sprintf("task %d", i)).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("%d-task builder: %s (%s per task)", genericBuilderTasks, elapsed, elapsed/genericBuilderTasks)
	if elapsed > genericBuilderBudget {
		t.Fatalf("took %s, budget %s", elapsed, genericBuilderBudget)
	}
}
