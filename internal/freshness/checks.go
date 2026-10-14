// This file owns the checks freshness makes around one Task's callback: is
// its Basis current, do its Verify checks hold, and were its tracked
// operations already current.

package freshness

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Narration hears what freshness judged about a Task, for the reader.
type Narration interface {
	// BasisJudged reports the verdict on a Task's declared Basis.
	BasisJudged(id record.TaskID, judgement BasisJudgement)
	// VerifierObserved reports how one Verify check ended, by its
	// registration order.
	VerifierObserved(id record.TaskID, index int, outcome VerifierOutcome)
}

// taskChecks are graph.Checks for one Task.
type taskChecks struct {
	run       *Run
	id        record.TaskID
	verifiers []Verifier
}

// Checks are the checks around id's callback. The Verify checks are the
// ones registered so far: Define freezes them.
func (r *Run) Checks(id record.TaskID) graph.Checks {
	return taskChecks{run: r, id: id, verifiers: r.tasks.Verifiers(id)}
}

// Current judges the Task's Basis as it starts and narrates the verdict. A
// Task with no Basis is never current.
func (c taskChecks) Current(ctx context.Context) (bool, error) {
	judgement, err := c.run.JudgeBasis(ctx, c.id, ModeFor(c.run.dryRun()))
	if err != nil || !judgement.Declared {
		return false, err
	}
	c.run.narration.BasisJudged(c.id, judgement)
	return judgement.Current, nil
}

// Verify runs the Task's Verify checks and narrates each outcome.
func (c taskChecks) Verify(ctx context.Context) (evaluated, satisfied bool, err error) {
	if len(c.verifiers) == 0 {
		return false, false, nil
	}
	satisfied, err = EvaluateVerifiers(ctx, c.verifiers, func(index int, outcome VerifierOutcome) {
		c.run.narration.VerifierObserved(c.id, index, outcome)
	})
	return true, satisfied, err
}

// Operations is how many tracked operations the Task ran, and whether the
// manifest proved every one current.
func (c taskChecks) Operations() (count int, allCurrent bool) {
	return c.run.tasks.OperationCount(c.id), c.run.tasks.AllCurrent(c.id)
}
