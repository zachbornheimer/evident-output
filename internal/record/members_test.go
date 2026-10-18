package record_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

func TestMembersInterleaveTasksAndCollectionsByDeclaration(t *testing.T) {
	col := record.TasksSnapshot{
		Tasks:       []record.TaskSnapshot{{Name: "t2", Declaration: 2}, {Name: "t4", Declaration: 4}},
		Collections: []record.TasksSnapshot{{Name: "c1", Declaration: 1}, {Name: "c3", Declaration: 3}},
	}
	var got []string
	for _, m := range col.Members() {
		if m.Task != nil {
			got = append(got, m.Task.Name)
		} else {
			got = append(got, m.Collection.Name)
		}
	}
	want := []string{"c1", "t2", "c3", "t4"}
	if len(got) != len(want) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("members = %v, want %v", got, want)
		}
	}
}
