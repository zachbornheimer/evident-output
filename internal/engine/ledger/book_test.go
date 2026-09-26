package ledger

import "testing"

// TestLedgerQualifiesEachSameNamedSectionOnce proves opening a section names
// only that section (and, when it first makes a name ambiguous, the one
// section already holding it): a Task's container path is fixed at
// declaration, so recomputing every earlier same-named section's subject
// on each open made n same-named sections cost O(n²).
func TestLedgerQualifiesEachSameNamedSectionOnce(t *testing.T) {
	var book Book
	var opened []*Section
	for i, repo := range []string{"repo 0", "repo 1", "repo 2"} {
		s, ok := book.Open(Owner{ID: repo, Name: "prune", Declaration: i, Containers: []string{repo}}, Changed, repo)
		if !ok {
			t.Fatalf("Open(%s) opened=false, want true", repo)
		}
		opened = append(opened, s)
		if len(opened) == 2 {
			for _, s := range opened {
				s.subject = "already qualified"
			}
		}
	}
	if len(opened) != 3 {
		t.Fatalf("opened %d sections, want 3", len(opened))
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

// TestBookOrdersSectionsByDeclarationTiesArrival proves Sections returns
// sections in owner declaration order, ties broken by arrival order.
func TestBookOrdersSectionsByDeclarationTiesArrival(t *testing.T) {
	var book Book
	third, _ := book.Open(Owner{ID: "c", Name: "c", Declaration: 5}, Changed, "c")
	first, _ := book.Open(Owner{ID: "a", Name: "a", Declaration: 1}, Changed, "a")
	tie1, _ := book.Open(Owner{ID: "b1", Name: "b1", Declaration: 3}, Changed, "b1")
	tie2, _ := book.Open(Owner{ID: "b2", Name: "b2", Declaration: 3}, Changed, "b2")

	got := book.Sections(Changed)
	want := []*Section{first, tie1, tie2, third}
	if len(got) != len(want) {
		t.Fatalf("Sections = %d entries, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("Sections[%d] = %q, want %q", i, got[i].id, s.id)
		}
	}
}

// TestBookOpenExistingReturnsFalse proves a second Open under the same
// owner and tense returns the existing section with opened=false.
func TestBookOpenExistingReturnsFalse(t *testing.T) {
	var book Book
	owner := Owner{ID: "a", Name: "a", Declaration: 1}
	first, opened := book.Open(owner, Changed, "a")
	if !opened {
		t.Fatalf("first Open opened=false, want true")
	}
	again, opened := book.Open(owner, Changed, "a")
	if opened {
		t.Errorf("second Open opened=true, want false")
	}
	if again != first {
		t.Errorf("second Open returned a different *Section")
	}
}

// TestBookRootKeepsBareSubjectAfterNestedSameNamedOpen proves a root
// owner's section keeps its bare name even once a nested, same-named
// section opens and becomes qualified.
func TestBookRootKeepsBareSubjectAfterNestedSameNamedOpen(t *testing.T) {
	var book Book
	root, _ := book.Open(Owner{ID: "root", Name: "prune", Declaration: 1}, Changed, "root")
	nested, _ := book.Open(Owner{ID: "nested", Name: "prune", Declaration: 2, Containers: []string{"repo"}}, Changed, "nested")

	if root.subject != "prune" {
		t.Errorf("root subject = %q, want bare %q", root.subject, "prune")
	}
	if want := "repo › prune"; nested.subject != want {
		t.Errorf("nested subject = %q, want %q", nested.subject, want)
	}
}
