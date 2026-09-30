//go:build evopending

package mcp_test

import "testing"

const childTasksInTaskDefineBody = `
func run(out *evo.Output, packages []string) {
	group := out.Group("centralize packages")
	group.Task("centralize packages").Define(func(ctx context.Context) error {
		for _, pkg := range packages {
			group.Task("centralize " + pkg).Define(func(ctx context.Context) error { return nil })
		}
		return nil
	})
}
`

const dynamicBuilderBody = `
func run(out *evo.Output, packages []string) {
	group := out.Group("centralize packages")
	group.Define(func(g *evo.GroupHandle) {
		for _, pkg := range packages {
			g.Task("centralize " + pkg).Define(func(ctx context.Context) error { return nil })
		}
	})
}
`

func TestCMCP_005_ChildTasksDeclaredInsideTaskDefineAreFlagged(t *testing.T) {
	requireFlagged(t, evoImport+childTasksInTaskDefineBody, "inside a Task", "Define", "Group")
	requireClean(t, evoImport+dynamicBuilderBody)
}

const ioInBuilderBody = `
func run(out *evo.Output) {
	group := out.Group("centralize packages")
	group.Define(func(g *evo.GroupHandle) {
		entries, _ := os.ReadDir("/opt/packages")
		for _, e := range entries {
			g.Task("centralize " + e.Name()).Define(func(ctx context.Context) error { return nil })
		}
	})
}
`

const goroutineInBuilderBody = `
func run(out *evo.Output) {
	group := out.Group("centralize packages")
	group.Define(func(g *evo.GroupHandle) {
		go poll()
		g.Task("centralize").Define(func(ctx context.Context) error { return nil })
	})
}
`

func TestCMCP_006_IOAndGoroutinesInsideTopologyBuilderAreFlagged(t *testing.T) {
	requireFlagged(t, evoImport+ioInBuilderBody, "builder", "I/O")
	requireFlagged(t, evoImport+goroutineInBuilderBody, "builder", "goroutine")
}

const presentationTaskBody = `
func run(out *evo.Output) {
	out.Task("Phase 2 header").Define(func(ctx context.Context) error {
		fmt.Println("== phase 2 ==")
		return nil
	})
}
`

func TestCMCP_007_PresentationOnlyTaskIsFlagged(t *testing.T) {
	requireFlagged(t, evoImport+presentationTaskBody, "presentation", "Task")
}

const giantLoopTaskBody = `
func run(out *evo.Output, branches []string) {
	out.Task("prune branches").Define(func(ctx context.Context) error {
		for _, b := range branches {
			if err := deleteBranch(ctx, b); err != nil {
				return err
			}
		}
		return nil
	})
}
`

const perItemTasksBody = `
func run(out *evo.Output, branches []string) {
	group := out.Group("prune branches")
	for _, b := range branches {
		group.Task("delete " + b).Define(func(ctx context.Context) error { return deleteBranch(ctx, b) })
	}
}
`

func TestCMCP_008_OneTaskLoopingOverItemsThatDeserveAnOutcomeIsFlagged(t *testing.T) {
	requireFlagged(t, evoImport+giantLoopTaskBody, "each item", "Group")
	requireClean(t, evoImport+perItemTasksBody)
}
