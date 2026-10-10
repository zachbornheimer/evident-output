// Fixture: none of API-064..071 may fire on the canonical zq prune shape
// (contract §31, "The intended zq shape"): Computes ordered by After or by
// Sequence position, a dynamic Group builder reading Get(), and Tasks that
// only do work.
package prune

import (
	"context"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
)

func declarePrune(out *evo.Output, m *fakeMachine) {
	prune := out.Group("prune")

	repository := prune.Group("repository")
	branches := evo.Compute(repository.Task("prune landed branches"), func(context.Context) ([]string, error) {
		m.record("branches")
		return m.branches, nil
	})
	worktrees := evo.Compute(repository.Task("prune unused worktrees"), func(context.Context) ([]string, error) {
		return []string{"/kept"}, nil
	})
	_ = worktrees
	repository.Task("prune stale remote-tracking refs").Define(func(context.Context) error { return nil })
	repository.Task("prune deleted remote branches").After(branches).Define(func(context.Context) error {
		m.record("remote:" + strings.Join(branches.Get(), ","))
		return nil
	})

	packages := prune.Sequence("consolidate packages")
	managers := evo.Compute(packages.Task("detect package managers").After(worktrees), func(context.Context) ([]string, error) {
		return m.managerNames(), nil
	})
	inventory := evo.Compute(packages.Task("discover installed packages"), func(context.Context) (map[string][]string, error) {
		return m.packages, nil
	})
	packages.Group("centralize packages").Define(func(centralize *evo.GroupHandle) {
		for _, manager := range managers.Get() {
			perManager := centralize.Group(manager)
			for _, pkg := range inventory.Get()[manager] {
				perManager.Task("centralize " + pkg).Define(func(context.Context) error {
					m.record("centralize:" + pkg)
					return nil
				})
			}
		}
	})
}
