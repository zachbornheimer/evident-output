package record

import "testing"

// TestLedgerQualifiesEachSameNamedSectionOnce proves opening a section
// names only that section (and, when it first makes a name ambiguous, the
// one section already holding it): a Task's container path is fixed at
// declaration, so recomputing every earlier same-named section's subject
// on each open made n same-named sections cost O(n²) under the run's lock.
func TestLedgerQualifiesEachSameNamedSectionOnce(t *testing.T) {
	var l ledger
	var opened []*ledgerSection
	for i, repo := range []string{"repo 0", "repo 1", "repo 2"} {
		owner := LedgerOwner{
			ID: "task_" + repo, Name: "prune", Declaration: i,
			Containers: ContainerPath{{ID: "group_" + repo, Name: repo}},
		}
		opened = append(opened, l.open(owner, TenseChanged, "changes_"+repo))
		if len(opened) == 2 {
			for _, s := range opened {
				s.subject = "already qualified"
			}
		}
	}
	for _, s := range opened[:2] {
		if s.subject != "already qualified" {
			t.Errorf("earlier section recomputed to %q on a later open", s.subject)
		}
	}
	if want := "repo 2 › prune"; opened[2].subject != want {
		t.Errorf("new section subject = %q, want %q", opened[2].subject, want)
	}
}

// TestLedgerKeepsARootSectionBare proves a root Task's section keeps its bare
// name even when a nested Task shares it: root names are unique siblings.
func TestLedgerKeepsARootSectionBare(t *testing.T) {
	var l ledger
	root := l.open(LedgerOwner{ID: "root", Name: "prune", Declaration: 1}, TenseChanged, "changes_1")
	nested := l.open(LedgerOwner{
		ID: "nested", Name: "prune", Declaration: 2,
		Containers: ContainerPath{{ID: "g", Name: "alpha"}},
	}, TenseChanged, "changes_2")
	if root.subject != "prune" {
		t.Errorf("root subject = %q, want the bare name", root.subject)
	}
	if want := "alpha › prune"; nested.subject != want {
		t.Errorf("nested subject = %q, want %q", nested.subject, want)
	}
}

// TestRunRecordEntryFixesTheIntendedVerbOnce proves the first entry names the
// section's intended verb and later entries leave it alone.
func TestRunRecordEntryFixesTheIntendedVerbOnce(t *testing.T) {
	run := NewRun()
	run.OpenSection(LedgerOwner{ID: "t", Name: "sync"}, TenseChanged, "changes_1")
	run.RecordEntry("t", TenseChanged, "deleted", CountedEntry("delete", "branch", 0))
	run.RecordEntry("t", TenseChanged, "created", CountedEntry("create", "tag", 2))
	section, ok := run.Section("t", TenseChanged)
	if !ok {
		t.Fatal("section not open")
	}
	if section.IntendedVerb != "delete" {
		t.Errorf("IntendedVerb = %q, want the first entry's verb", section.IntendedVerb)
	}
	if len(section.Records) != 1 || section.Records[0].Verb != "created" {
		t.Errorf("Records = %+v, want only the non-zero created row", section.Records)
	}
}
