package project

import (
	"reflect"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestTally(t *testing.T) {
	t.Parallel()
	rec := func(reason, name string, causes ...string) record.TaxonomyRecord {
		return record.TaxonomyRecord{Reason: reason, Name: name, Causes: causes}
	}
	tests := []struct {
		name    string
		records []record.TaxonomyRecord
		reasons []ReasonTally
		causes  []string
	}{
		{name: "empty"},
		{
			name:    "one reason keeps record order",
			records: []record.TaxonomyRecord{rec("protected", "main"), rec("protected", "develop")},
			reasons: []ReasonTally{{Reason: "protected", Names: []string{"main", "develop"}}},
		},
		{
			name: "reasons in first-seen order, not sorted",
			records: []record.TaxonomyRecord{
				rec("unpushed", "feat/a"), rec("protected", "main"), rec("unpushed", "feat/b"), rec("dirty", "wip"),
			},
			reasons: []ReasonTally{
				{Reason: "unpushed", Names: []string{"feat/a", "feat/b"}},
				{Reason: "protected", Names: []string{"main"}},
				{Reason: "dirty", Names: []string{"wip"}},
			},
		},
		{
			name: "causes concatenate in record order across reasons",
			records: []record.TaxonomyRecord{
				rec("offline", "origin", "dial tcp: timeout", "retry exhausted"), rec("--skip-fetch", "upstream"),
				rec("offline", "mirror", "dns: no such host"),
			},
			reasons: []ReasonTally{
				{Reason: "offline", Names: []string{"origin", "mirror"}},
				{Reason: "--skip-fetch", Names: []string{"upstream"}},
			},
			causes: []string{"dial tcp: timeout", "retry exhausted", "dns: no such host"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tally := TallyOf(tt.records)
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

func TestTallyCloneDoesNotShareBacking(t *testing.T) {
	t.Parallel()
	var orig Tally
	orig.Add(record.TaxonomyRecord{Reason: "pinned", Name: "a", Causes: []string{"why"}})
	clone := orig.Clone()
	orig.Add(record.TaxonomyRecord{Reason: "pinned", Name: "b"})
	orig.Add(record.TaxonomyRecord{Reason: "vendored", Name: "c", Causes: []string{"elsewhere"}})
	if clone.Total() != 1 {
		t.Fatalf("clone Total() = %d, want 1", clone.Total())
	}
	if got := clone.Reasons(); !reflect.DeepEqual(got, []ReasonTally{{Reason: "pinned", Names: []string{"a"}}}) {
		t.Fatalf("clone Reasons() = %#v", got)
	}
	if got := clone.Causes(); !reflect.DeepEqual(got, []string{"why"}) {
		t.Fatalf("clone Causes() = %v, want [why]", got)
	}
}

func TestDispositions_AddTaskSumsBothTallies(t *testing.T) {
	t.Parallel()
	var d Dispositions
	if !d.Empty() {
		t.Fatal("zero Dispositions must be Empty")
	}
	skipped := []record.TaxonomyRecord{{Reason: "--skip-fetch", Name: "remote-tracking"}}
	kept := []record.TaxonomyRecord{{Reason: "protected", Name: "main"}}
	d.AddTask(&record.TaskSnapshot{Name: "remote-tracking", Skipped: skipped})
	d.AddTask(&record.TaskSnapshot{Name: "main", Kept: kept})
	d.AddTask(&record.TaskSnapshot{Name: "develop", Kept: []record.TaxonomyRecord{{Reason: "protected", Name: "develop"}}})
	if d.Empty() || d.Skipped.Total() != 1 || d.Kept.Total() != 2 {
		t.Fatalf("got skipped %d kept %d, want 1 and 2", d.Skipped.Total(), d.Kept.Total())
	}
	if names := d.Kept.Reasons()[0].Names; !reflect.DeepEqual(names, []string{"main", "develop"}) {
		t.Fatalf("kept names = %v, want [main develop]", names)
	}
}
