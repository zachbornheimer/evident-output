package main

import (
	"context"
	"os"

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

	repository := prune.Group("repository")
	branches := evo.Compute(repository.Task("prune landed branches"), fixture.LandedBranches)
	worktrees := evo.Compute(repository.Task("prune unused worktrees"), fixture.UnusedWorktrees)
	repository.Task("prune stale remote-tracking refs").Define(fixture.PruneStaleRemoteRefs)
	if fixture.IncludeRemote() {
		repository.Task("prune deleted remote branches").After(branches).Define(func(ctx context.Context) error {
			return fixture.DeleteRemoteBranches(ctx, branches.Get())
		})
	}

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
