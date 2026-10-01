package runner

import (
	"fmt"
	"io"
	"slices"

	"github.com/zachbornheimer/evident-output/eval/driver"
	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

// WorstCaseContextTokens is the prompt size assumed for every turn when
// bounding the cost of a run: the 200k-token context window, billed as
// uncached input on every turn.
const WorstCaseContextTokens = 200_000

// Plan is what a run would do and the most it could cost.
type Plan struct {
	Tasks        []string
	Models       []string
	Samples      int
	WorstCaseUSD float64
}

// SelectTasks picks the tasks a run covers: the named ids, or every task the
// current API can express. Unknown or blocked ids are errors.
func SelectTasks(all []evaltask.Task, ids []string, allReady bool) ([]evaltask.Task, error) {
	var picked []evaltask.Task
	for _, task := range all {
		wanted := slices.Contains(ids, task.ID)
		if wanted && task.Blocked() {
			return nil, fmt.Errorf("select task %s: blocked on %v", task.ID, task.Expect.BlockedAPI)
		}
		if wanted || (allReady && !task.Blocked()) {
			picked = append(picked, task)
		}
	}
	for _, id := range ids {
		if !slices.ContainsFunc(picked, func(task evaltask.Task) bool { return task.ID == id }) {
			return nil, fmt.Errorf("select task %q: no such task", id)
		}
	}
	if len(picked) == 0 {
		return nil, fmt.Errorf("select tasks: none matched")
	}
	return picked, nil
}

// BuildPlan computes the worst-case dollars: every sample uses every turn,
// each turn emits max_tokens and reads a full context uncached. It fails for
// a model whose price is unverified and not overridden.
func BuildPlan(cfg Config, tasks []evaltask.Task) (Plan, error) {
	plan := Plan{Models: cfg.Models, Samples: cfg.Samples}
	for _, task := range tasks {
		plan.Tasks = append(plan.Tasks, task.ID)
	}
	perTurn := driver.Usage{InputTokens: WorstCaseContextTokens, OutputTokens: driver.DefaultMaxTokens}
	for _, model := range cfg.Models {
		price, err := cfg.Prices.Resolve(model)
		if err != nil {
			return Plan{}, fmt.Errorf("%w: %w", ErrRefusedToStart, err)
		}
		turns := float64(len(tasks) * cfg.Samples * driver.MaxToolTurns)
		plan.WorstCaseUSD += turns * price.Cost(perTurn)
	}
	return plan, nil
}

// Print writes the plan for a human.
func (p Plan) Print(out io.Writer, maxUSD float64) {
	fmt.Fprintf(out, "tasks:   %v\nmodels:  %v\nsamples: %d per task and model\n", p.Tasks, p.Models, p.Samples)
	fmt.Fprintf(out, "worst case: $%.2f (every sample uses all %d turns at max_tokens %d over a %d-token context, no caching)\n",
		p.WorstCaseUSD, driver.MaxToolTurns, driver.DefaultMaxTokens, WorstCaseContextTokens)
	if maxUSD > 0 {
		fmt.Fprintf(out, "spend cap: $%.2f (the run aborts when accumulated cost reaches it)\n", maxUSD)
	}
}
