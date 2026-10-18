package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

const (
	memberOrderFirst  = "step one: build"
	memberOrderSecond = "step two: publish"
)

// declareGroupThenTask declares a nested Group, then a Task, in one
// Sequence: the order CONTRACT.md says the members are shown in.
func declareGroupThenTask(_ context.Context, r *evo.Run) error {
	s := r.Sequence("release")
	s.Group(memberOrderFirst).Task("compile").Define(func(context.Context) error { return nil })
	s.Task(memberOrderSecond).Define(func(context.Context) error { return nil })
	return nil
}

// TestContainerMembersRenderInDeclarationOrder pins CONTRACT.md §3 and §8: a
// Group or Sequence shows its members, Tasks and nested collections alike, in
// the order they were declared, in Plain, in the Human final screen and in
// the evo.run children list.
func TestContainerMembersRenderInDeclarationOrder(t *testing.T) {
	for _, mode := range []evo.Output{evo.Plain, evo.Human} {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Output: mode}, declareGroupThenTask)
			first, second := strings.Index(run.Text(), memberOrderFirst), strings.Index(run.Text(), memberOrderSecond)
			if first < 0 || second < 0 || first > second {
				t.Errorf("%q must be listed before %q:\n%s", memberOrderFirst, memberOrderSecond, run.Text())
			}
		})
	}

	t.Run("json children", func(t *testing.T) {
		run := executeContract(context.Background(), t,
			evo.Config{Output: evo.JSON, State: evo.State{Dir: t.TempDir(), AppID: "contract"}}, declareGroupThenTask)
		data, _ := runDocument(t, run.Stdout)["data"].(map[string]any)
		names := map[string]string{}
		for _, key := range []string{"tasks", "collections"} {
			list, _ := data[key].([]any)
			for _, item := range list {
				m, _ := item.(map[string]any)
				id, _ := m["id"].(string)
				names[id], _ = m["name"].(string)
			}
		}
		var got []string
		collections, _ := data["collections"].([]any)
		for _, item := range collections {
			m, _ := item.(map[string]any)
			if m["name"] != "release" {
				continue
			}
			children, _ := m["children"].([]any)
			for _, c := range children {
				id, _ := c.(string)
				got = append(got, names[id])
			}
		}
		if len(got) != 2 || got[0] != memberOrderFirst || got[1] != memberOrderSecond {
			t.Errorf("release children = %q, want [%q %q]", got, memberOrderFirst, memberOrderSecond)
		}
	})
}
