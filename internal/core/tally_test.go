package core_test

import (
	"reflect"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
)

func TestTally(t *testing.T) {
	t.Parallel()
	rec := func(reason, name string, causes ...string) core.TaxonomyRecord {
		return core.TaxonomyRecord{Reason: reason, Name: name, Causes: causes}
	}
	tests := []struct {
		name    string
		records []core.TaxonomyRecord
		reasons []core.ReasonTally
		causes  []string
	}{
		{name: "empty"},
		{
			name:    "one reason keeps record order",
			records: []core.TaxonomyRecord{rec("protected", "main"), rec("protected", "develop")},
			reasons: []core.ReasonTally{{Reason: "protected", Names: []string{"main", "develop"}}},
		},
		{
			name: "reasons in first-seen order, not sorted",
			records: []core.TaxonomyRecord{
				rec("unpushed", "feat/a"), rec("protected", "main"), rec("unpushed", "feat/b"), rec("dirty", "wip"),
			},
			reasons: []core.ReasonTally{
				{Reason: "unpushed", Names: []string{"feat/a", "feat/b"}},
				{Reason: "protected", Names: []string{"main"}},
				{Reason: "dirty", Names: []string{"wip"}},
			},
		},
		{
			name: "causes concatenate in record order across reasons",
			records: []core.TaxonomyRecord{
				rec("offline", "origin", "dial tcp: timeout", "retry exhausted"), rec("--skip-fetch", "upstream"),
				rec("offline", "mirror", "dns: no such host"),
			},
			reasons: []core.ReasonTally{
				{Reason: "offline", Names: []string{"origin", "mirror"}},
				{Reason: "--skip-fetch", Names: []string{"upstream"}},
			},
			causes: []string{"dial tcp: timeout", "retry exhausted", "dns: no such host"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tally := core.TallyOf(tt.records)
			if got := tally.Reasons(); !reflect.DeepEqual(got, tt.reasons) {
				t.Fatalf("Reasons() = %#v, want %#v", got, tt.reasons)
			}
			if got := tally.Causes(); !reflect.DeepEqual(got, tt.causes) {
				t.Fatalf("Causes() = %#v, want %#v", got, tt.causes)
			}
			if tally.Total() != len(tt.records) {
				t.Fatalf("Total() = %d, want %d", tally.Total(), len(tt.records))
			}
			sum := 0
			for _, part := range tally.Reasons() {
				sum += len(part.Names)
			}
			if sum != tally.Total() {
				t.Fatalf("reason parts sum to %d, Total() is %d", sum, tally.Total())
			}
		})
	}
}

func TestDispositions_AddTaskSumsBothTallies(t *testing.T) {
	t.Parallel()
	var d core.Dispositions
	if !d.Empty() {
		t.Fatal("zero Dispositions must be Empty")
	}
	skipped := []core.TaxonomyRecord{{Reason: "--skip-fetch", Name: "remote-tracking"}}
	kept := []core.TaxonomyRecord{{Reason: "protected", Name: "main"}}
	d.AddTask(&core.TaskSnapshot{Name: "remote-tracking", Skipped: skipped})
	d.AddTask(&core.TaskSnapshot{Name: "main", Kept: kept})
	d.AddTask(&core.TaskSnapshot{Name: "develop", Kept: []core.TaxonomyRecord{{Reason: "protected", Name: "develop"}}})
	if d.Empty() || d.Skipped.Total() != 1 || d.Kept.Total() != 2 {
		t.Fatalf("got skipped %d kept %d, want 1 and 2", d.Skipped.Total(), d.Kept.Total())
	}
	if names := d.Kept.Reasons()[0].Names; !reflect.DeepEqual(names, []string{"main", "develop"}) {
		t.Fatalf("kept names = %v, want [main develop]", names)
	}
}
