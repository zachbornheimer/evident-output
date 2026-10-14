package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestTallyForgetsASuccessThatSettledAgainFailed(t *testing.T) {
	g := newGraph()
	group := g.AddContainer(nil, "group", false)
	first := must(g.AddTask(group, "first", record.TaskInit{State: record.Pending}))
	second := must(g.AddTask(group, "second", record.TaskInit{State: record.Pending}))
	for _, member := range []*Task{first, second} {
		member.Rec.Settle(record.Succeeded)
		group.tally.settle(member)
	}
	if group.tally.succeededPrefix != 2 {
		t.Fatalf("succeededPrefix = %d after both members succeeded, want 2", group.tally.succeededPrefix)
	}

	first.Rec.Settle(record.Failed)
	group.tally.settle(first)

	if got := group.tally.succeededPrefix; got != 0 {
		t.Errorf("succeededPrefix = %d after the first member failed, want 0", got)
	}
	if !group.tally.failedThrough(first.Declaration) {
		t.Error("group does not read the first member as failed")
	}
}
