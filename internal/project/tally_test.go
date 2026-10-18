package project

import (
	"reflect"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// excludedItem is an Excluded item: a Task Excluded for reason, with the
// Summary and Problem Exclude records.
func excludedItem(name, reason string, facts ...record.Fact) *record.TaskSnapshot {
	return &record.TaskSnapshot{
		Name: name, State: record.Excluded, Summary: reason, Facts: facts,
		Problems: []record.Problem{{Summary: reason}},
	}
}

func TestTally_AddItemPartitionsByReasonInFirstSeenOrder(t *testing.T) {
	t.Parallel()
	var tally Tally
	if !tally.Empty() {
		t.Fatal("zero Tally must be Empty")
	}
	for _, item := range []struct{ name, reason string }{
		{"feat/a", "unpushed"}, {"main", "protected"}, {"feat/b", "unpushed"}, {"wip", "dirty"}, {"develop", "protected"},
	} {
		tally.AddItem(excludedItem(item.name, item.reason))
	}
	want := []ReasonTally{
		{Reason: "unpushed", Names: []string{"feat/a", "feat/b"}},
		{Reason: "protected", Names: []string{"main", "develop"}},
		{Reason: "dirty", Names: []string{"wip"}},
	}
	if got := tally.Reasons(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Reasons() = %#v, want %#v", got, want)
	}
	sum := 0
	for _, part := range tally.Reasons() {
		sum += len(part.Names)
	}
	if tally.Total() != 5 || sum != tally.Total() {
		t.Fatalf("Total() = %d with reason parts summing to %d, want 5 and 5", tally.Total(), sum)
	}
}

func TestTally_AddItemKeepsFactsAlignedWithNames(t *testing.T) {
	t.Parallel()
	var tally Tally
	tally.AddItem(excludedItem("a", "pinned"))
	tally.AddItem(excludedItem("b", "pinned", record.Fact{Name: "sha", Value: "abc"}))
	part := tally.Reasons()[0]
	if !part.HasFacts() || len(part.Facts) != len(part.Names) || len(part.Facts[0]) != 0 || len(part.Facts[1]) != 1 {
		t.Fatalf("Facts = %#v for Names %v, want one slot per name with only b's set", part.Facts, part.Names)
	}
}

func TestTally_CloneDoesNotShareBacking(t *testing.T) {
	t.Parallel()
	var orig Tally
	orig.AddItem(excludedItem("a", "pinned"))
	clone := orig.Clone()
	orig.AddItem(excludedItem("b", "pinned"))
	orig.AddItem(excludedItem("c", "vendored"))
	if clone.Total() != 1 {
		t.Fatalf("clone Total() = %d, want 1", clone.Total())
	}
	if got := clone.Reasons(); !reflect.DeepEqual(got, []ReasonTally{{Reason: "pinned", Names: []string{"a"}}}) {
		t.Fatalf("clone Reasons() = %#v", got)
	}
}

func TestExcludeReason_OnlyABareReasonFolds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		task record.TaskSnapshot
		want bool
	}{
		{"bare reason", *excludedItem("a", "protected"), true},
		{"a Detail keeps the row", record.TaskSnapshot{State: record.Excluded, Problems: []record.Problem{{Summary: "protected", Detail: "why"}}}, false},
		{"a second Problem keeps the row", record.TaskSnapshot{State: record.Excluded, Problems: []record.Problem{{Summary: "protected"}, {Summary: "also"}}}, false},
		{"another outcome", record.TaskSnapshot{State: record.Failed, Problems: []record.Problem{{Summary: "protected"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, got := ExcludeReason(&tt.task); got != tt.want {
				t.Fatalf("ExcludeReason ok = %v, want %v", got, tt.want)
			}
		})
	}
}
