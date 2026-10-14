package mcp_test

import "testing"

const childTasksInTaskDefineBody = `
func run(r *evo.Run, packages []string) {
	group := r.Group("centralize packages")
	group.Task("centralize packages").Define(func(ctx context.Context) error {
		for _, pkg := range packages {
			group.Task("centralize " + pkg).Define(func(ctx context.Context) error { return nil })
		}
		return nil
	})
}
`

const dynamicBuilderBody = `
func run(r *evo.Run, packages []string) {
	group := r.Group("centralize packages")
	group.Build(func(g *evo.Group) {
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
func run(r *evo.Run) {
	group := r.Group("centralize packages")
	group.Build(func(g *evo.Group) {
		entries, _ := os.ReadDir("/opt/packages")
		for _, e := range entries {
			g.Task("centralize " + e.Name()).Define(func(ctx context.Context) error { return nil })
		}
	})
}
`

const goroutineInBuilderBody = `
func run(r *evo.Run) {
	group := r.Group("centralize packages")
	group.Build(func(g *evo.Group) {
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
func run(r *evo.Run) {
	r.Task("Phase 2 header").Define(func(ctx context.Context) error {
		fmt.Println("== phase 2 ==")
		return nil
	})
}
`

func TestCMCP_007_PresentationOnlyTaskIsFlagged(t *testing.T) {
	requireFlagged(t, evoImport+presentationTaskBody, "presentation", "Task")
}

const giantLoopTaskBody = `
func run(r *evo.Run, branches []string) {
	r.Task("prune branches").Define(func(ctx context.Context) error {
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
func run(r *evo.Run, branches []string) {
	group := r.Group("prune branches")
	for _, b := range branches {
		group.Task("delete " + b).Define(func(ctx context.Context) error { return deleteBranch(ctx, b) })
	}
}
`

func TestCMCP_008_OneTaskLoopingOverItemsThatDeserveAnOutcomeIsFlagged(t *testing.T) {
	requireFlagged(t, evoImport+giantLoopTaskBody, "each item", "Group")
	requireClean(t, evoImport+perItemTasksBody)
}
