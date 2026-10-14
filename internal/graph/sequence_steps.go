package graph

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// StepStarted notes t, declared directly in the Sequence c, becoming Running,
// and reports whether another of c's steps was still Running: a Sequence runs
// one step at a time. It keeps only the steps that may still be Running, so
// it never rescans every step. Callers serialize calls for one container.
func (c *Container) StepStarted(t *Task) (concurrent bool) {
	c.runningSteps = slices.DeleteFunc(c.runningSteps, func(s *Task) bool { return s.Rec.State() != record.Running })
	concurrent = len(c.runningSteps) > 0
	c.runningSteps = append(c.runningSteps, t)
	return concurrent
}
