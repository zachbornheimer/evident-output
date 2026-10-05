package mcp_test

import (
	"strings"
	"testing"
)

const unorderedGetBody = `
func run(out *evo.Output) {
	repo := out.Group("repository")
	repo.Task("prune stale remote-tracking refs").Define(pruneRefs)
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	report := out.Sequence("report")
	report.Task("print branches").Define(func(ctx context.Context) error {
		return print(branches.Get())
	})
}
`

const orderedGetBody = `
func run(out *evo.Output) {
	repo := out.Group("repository")
	repo.Task("prune stale remote-tracking refs").Define(pruneRefs)
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	report := out.Sequence("report")
	report.Task("print branches").After(branches).Define(func(ctx context.Context) error {
		return print(branches.Get())
	})
}
`

func TestCMCP_001_UnorderedGetIsFlaggedWithMissingAfterSuggestion(t *testing.T) {
	f := requireFlagged(t, evoImport+unorderedGetBody, "Get", "After")
	if !strings.Contains(f.Suggestion, "After(branches)") {
		t.Fatalf("suggestion must name the missing edge After(branches): %q", f.Suggestion)
	}
	requireClean(t, evoImport+orderedGetBody)
}

const redundantAfterBody = `
func run(out *evo.Output) {
	steps := out.Sequence("consolidate packages")
	managers := evo.Compute(steps.Task("detect package managers"), detect)
	steps.Task("discover installed packages").After(managers).Define(func(ctx context.Context) error {
		return discover(managers.Get())
	})
}
`

const sequenceOrderedBody = `
func run(out *evo.Output) {
	steps := out.Sequence("consolidate packages")
	managers := evo.Compute(steps.Task("detect package managers"), detect)
	steps.Task("discover installed packages").Define(func(ctx context.Context) error {
		return discover(managers.Get())
	})
}
`

func TestCMCP_002_RedundantAfterInsideSequenceIsFlaggedAndNeverAutoAdded(t *testing.T) {
	requireFlagged(t, evoImport+redundantAfterBody, "redundant", "Sequence", "After")
	requireClean(t, evoImport+sequenceOrderedBody)
}

const outerVariableHandoffBody = `
func run(out *evo.Output) {
	var inventory Inventory
	discover := out.Task("discover installed packages")
	discover.Define(func(ctx context.Context) error {
		var err error
		inventory, err = discoverPackages(ctx)
		return err
	})
	out.Task("centralize packages").After(discover).Define(func(ctx context.Context) error {
		return centralize(ctx, inventory)
	})
}
`

func TestCMCP_003_OuterMutableVariableHandoffIsFlaggedTowardCompute(t *testing.T) {
	requireFlagged(t, evoImport+outerVariableHandoffBody, "Compute")
}

const intoPlumbingBody = `
func run(out *evo.Output) {
	var inventory Inventory
	out.Task("discover installed packages").Into(&inventory).Define(discoverPackages)
}
`

func TestCMCP_004_IntoResultPlumbingIsFlaggedTowardCompute(t *testing.T) {
	requireFlagged(t, evoImport+intoPlumbingBody, "Into", "Compute")
}
