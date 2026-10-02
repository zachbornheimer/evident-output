package compose_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// fakeMachine is the in-memory I/O behind the prune topology: what the
// repository and the package managers would report, and a record of what
// the run did to them.
type fakeMachine struct {
	mu       sync.Mutex
	branches []string
	packages map[string][]string
	actions  []string
}

func (m *fakeMachine) record(action string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, action)
}

func (m *fakeMachine) managerNames() []string {
	return []string{"composer", "npm"}
}

// declarePrune declares Zach's settled zq prune shape (contract §31, "The
// intended zq shape") onto out.
func declarePrune(out *evo.Output, m *fakeMachine) {
	prune := out.Group("prune")

	repository := prune.Group("repository")
	branches := evo.Compute(repository.Task("prune landed branches"), func(context.Context) ([]string, error) {
		m.record("branches")
		return m.branches, nil
	})
	worktrees := evo.Compute(repository.Task("prune unused worktrees"), func(context.Context) ([]string, error) {
		m.record("worktrees")
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
		m.record("managers")
		return m.managerNames(), nil
	})
	inventory := evo.Compute(packages.Task("discover installed packages"), func(context.Context) (map[string][]string, error) {
		m.record("inventory")
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

const expectedPruneTree = `prune [done]
  consolidate packages [done]
    centralize packages [done]
      composer [done]
        centralize psr/log@3.0.0 [done]
      npm [done]
        centralize debug@4.3.4 [done]
        centralize lodash@4.17.21 [done]
    detect package managers [done]
    discover installed packages [done]
  repository [done]
    prune deleted remote branches [done]
    prune landed branches [done]
    prune stale remote-tracking refs [done]
    prune unused worktrees [done]
`

func TestC31_018_ZqPruneTopologyBuildsRunsAndYieldsExpectedTree(t *testing.T) {
	out := newQuietOutput(t, true)
	machine := &fakeMachine{
		branches: []string{"old"},
		packages: map[string][]string{
			"composer": {"psr/log@3.0.0"},
			"npm":      {"debug@4.3.4", "lodash@4.17.21"},
		},
	}
	declarePrune(out, machine)
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	snap := out.Snapshot()
	if len(snap.Collections) != 1 {
		t.Fatalf("top-level containers = %d, want 1 (prune)", len(snap.Collections))
	}
	if got := renderTree(snap.Collections[0]); got != expectedPruneTree {
		t.Fatalf("tree mismatch\ngot:\n%s\nwant:\n%s", got, expectedPruneTree)
	}
	assertBefore(t, machine.actions, "worktrees", "managers")
	assertBefore(t, machine.actions, "managers", "inventory")
	assertBefore(t, machine.actions, "inventory", "centralize:debug@4.3.4")
	assertBefore(t, machine.actions, "branches", "remote:old")
}

func assertBefore(t *testing.T, actions []string, first, second string) {
	t.Helper()
	index := map[string]int{}
	for i, a := range actions {
		index[a] = i
	}
	a, aok := index[first]
	b, bok := index[second]
	if !aok || !bok || a > b {
		t.Fatalf("want %q before %q in %v", first, second, actions)
	}
}

func TestC31_023_PlainOutputHidesGroupHeaderUnderSequence(t *testing.T) {
	out, buf := newPlainOutput(t)
	steps := out.Sequence("steps")
	steps.Task("read plan").Define(func(context.Context) error { return nil })
	validate := steps.Group("validate plan")
	validate.Task("check schema").Define(func(context.Context) error { return nil })
	validate.Task("check references").Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "validate plan") {
		t.Fatalf("plain output renders the Group header:\n%s", got)
	}
	if !strings.Contains(got, "check schema") || !strings.Contains(got, "check references") {
		t.Fatalf("plain output lost the Group's children:\n%s", got)
	}
}
