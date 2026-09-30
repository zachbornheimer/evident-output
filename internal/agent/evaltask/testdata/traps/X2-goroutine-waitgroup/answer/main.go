package main

import (
	"context"
	"os"
	"sync"

	evo "github.com/zachbornheimer/evident-output"

	"evalsandbox/fixture"
)

func main() {
	evo.Init(evo.Config{Title: "prune"})
	os.Exit(evo.Main(run))
}

func run(context.Context) error {
	declarePrune(evo.Default())
	return nil
}

func declarePrune(out *evo.Output) {
	prune := out.Group("prune")

	var wg sync.WaitGroup
	prune.Task("prune repository").Define(func(ctx context.Context) error {
		wg.Add(3)
		go func() { defer wg.Done(); _, _ = fixture.LandedBranches(ctx) }()
		go func() { defer wg.Done(); _, _ = fixture.UnusedWorktrees(ctx) }()
		go func() { defer wg.Done(); _ = fixture.PruneStaleRemoteRefs(ctx) }()
		wg.Wait()
		return nil
	})

	worktrees := evo.Compute(prune.Task("list worktrees"), fixture.UnusedWorktrees)

	packages := prune.Sequence("consolidate packages")
	managers := evo.Compute(packages.Task("detect package managers").After(worktrees), func(ctx context.Context) ([]string, error) {
		return fixture.DetectPackageManagers(ctx, worktrees.Get())
	})
	inventory := evo.Compute(packages.Task("discover installed packages"), fixture.InstalledPackages)
	packages.Group("centralize packages").Define(func(centralize *evo.GroupHandle) {
		for _, manager := range managers.Get() {
			perManager := centralize.Group(manager)
			for _, pkg := range inventory.Get()[manager] {
				perManager.Task("centralize " + pkg).Define(func(ctx context.Context) error {
					return fixture.Centralize(ctx, pkg)
				})
			}
		}
	})
}
