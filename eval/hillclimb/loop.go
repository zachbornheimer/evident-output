package hillclimb

import (
	"context"
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/eval/scoreboard"
)

// Why the climb ended.
const (
	StopSaturated   = "training pass^k reached 100%"
	StopHeldOut     = "held-out showed no gain on consecutive checks"
	StopSpendCap    = "dollar cap reached"
	StopStepLimit   = "step limit reached"
	rejectedByGuard = "guard"
	rejectedByCheck = "checks"
	rejectedByScore = "score"
	acceptedStep    = "accepted"
)

// Worker proposes one edit to a tuned surface by changing the working tree.
// Real agents implement it; tests use a fake.
type Worker interface {
	ProposeEdit(ctx context.Context, failures []Failure) error
}

// Workspace is the working tree under edit.
type Workspace interface {
	Edit() (Edit, error)
	Revert() error
	Commit(summary string) error
}

// Checker runs the repo's gate (the mise ci equivalent); a non-nil error
// means the edit broke something.
type Checker interface {
	Run(ctx context.Context) error
}

// Evaluator runs the tasks and reports dollars spent.
type Evaluator interface {
	Training(ctx context.Context) (Standing, error)
	HeldOut(ctx context.Context) (scoreboard.Aggregate, float64, error)
}

// Loop is one hill-climb.
type Loop struct {
	Worker    Worker
	Workspace Workspace
	Guard     Guard
	Checker   Checker
	Evaluator Evaluator
	// MaxUSD and MaxSteps bound the climb.
	MaxUSD   float64
	MaxSteps int
}

// Step is one worker edit and what happened to it.
type Step struct {
	Outcome string
	Reason  string
}

// Result is the whole climb.
type Result struct {
	Steps    []Step
	Stop     string
	SpentUSD float64
	Training Standing
}

// Run climbs until training saturates, held-out stops improving, the dollar
// cap is hit, or MaxSteps is reached.
func (l Loop) Run(ctx context.Context) (Result, error) {
	standing, err := l.Evaluator.Training(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("measure baseline training: %w", err)
	}
	result := Result{Training: standing, SpentUSD: standing.CostUSD}
	held := heldOutWatch{}
	for len(result.Steps) < l.MaxSteps {
		if stop := l.stopReason(result, held); stop != "" {
			result.Stop = stop
			return result, nil
		}
		step, next, err := l.attempt(ctx, result.Training)
		if err != nil {
			return result, fmt.Errorf("step %d: %w", len(result.Steps)+1, err)
		}
		result.SpentUSD += next.CostUSD
		result.Steps = append(result.Steps, step)
		if step.Outcome != acceptedStep {
			continue
		}
		result.Training = next
		aggregate, cost, err := l.Evaluator.HeldOut(ctx)
		if err != nil {
			return result, fmt.Errorf("step %d held-out check: %w", len(result.Steps), err)
		}
		result.SpentUSD += cost
		held.observe(aggregate)
	}
	result.Stop = StopStepLimit
	return result, nil
}

func (l Loop) stopReason(result Result, held heldOutWatch) string {
	switch {
	case result.Training.Saturated():
		return StopSaturated
	case held.stale >= HeldOutPatience:
		return StopHeldOut
	case result.SpentUSD >= l.MaxUSD:
		return StopSpendCap
	}
	return ""
}

// attempt asks for an edit, judges it, and keeps or reverts it.
func (l Loop) attempt(ctx context.Context, before Standing) (Step, Standing, error) {
	if err := l.Worker.ProposeEdit(ctx, before.Failures); err != nil {
		return Step{}, Standing{}, fmt.Errorf("propose edit: %w", err)
	}
	edit, err := l.Workspace.Edit()
	if err != nil {
		return Step{}, Standing{}, fmt.Errorf("read edit: %w", err)
	}
	if violations := l.Guard.Check(edit); len(violations) > 0 {
		return l.reject(rejectedByGuard, strings.Join(violations, "; "), Standing{})
	}
	if err := l.Checker.Run(ctx); err != nil {
		return l.reject(rejectedByCheck, err.Error(), Standing{})
	}
	after, err := l.Evaluator.Training(ctx)
	if err != nil {
		return Step{}, Standing{}, fmt.Errorf("measure training: %w", err)
	}
	decision := Accept(before, after)
	if !decision.Accepted {
		return l.reject(rejectedByScore, decision.Reason, after)
	}
	if err := l.Workspace.Commit(decision.Reason); err != nil {
		return Step{}, Standing{}, fmt.Errorf("commit accepted edit: %w", err)
	}
	return Step{Outcome: acceptedStep, Reason: decision.Reason}, after, nil
}

func (l Loop) reject(outcome, reason string, measured Standing) (Step, Standing, error) {
	if err := l.Workspace.Revert(); err != nil {
		return Step{}, Standing{}, fmt.Errorf("revert rejected edit (%s): %w", outcome, err)
	}
	return Step{Outcome: outcome, Reason: reason}, Standing{CostUSD: measured.CostUSD}, nil
}

// heldOutWatch counts consecutive held-out checks that did not beat the best.
type heldOutWatch struct {
	best  float64
	seen  bool
	stale int
}

func (w *heldOutWatch) observe(aggregate scoreboard.Aggregate) {
	if !w.seen || aggregate.PassAt1 > w.best {
		w.best, w.seen, w.stale = aggregate.PassAt1, true, 0
		return
	}
	w.stale++
}
