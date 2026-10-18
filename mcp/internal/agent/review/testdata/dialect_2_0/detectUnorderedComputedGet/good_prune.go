// The canonical prune topology: Computeds ordered by After or by Sequence
// position, a dynamic Build reading Get(), and Tasks that only do work.
package prune

import (
	"context"
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type machine struct {
	branches []string
	packages map[string][]string
	log      []string
}

func (m *machine) record(entry string) { m.log = append(m.log, entry) }
func (m *machine) managerNames() []string {
	names := make([]string, 0, len(m.packages))
	for name := range m.packages {
		names = append(names, name)
	}
	return names
}

func declarePrune(r *evo.Run, m *machine) {
	prune := r.Group("prune")

	repository := prune.Group("repository")
	branches := evo.Compute(repository.Task("prune landed branches"), func(context.Context) ([]string, error) {
		m.record("branches")
		return m.branches, nil
	})
	worktrees := evo.Compute(repository.Task("prune unused worktrees"), func(context.Context) ([]string, error) {
		return []string{"/kept"}, nil
	})
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
	packages.Group("centralize packages").Build(func(centralize *evo.Group) {
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
