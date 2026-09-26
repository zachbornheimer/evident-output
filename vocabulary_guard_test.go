package evo_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/vocabulary"
	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

// TestVocabularyGuard_FlagsUnclassifiedRemovedOrUnexportedNames proves
// internal/agent/vocabulary.Check is the single source that keeps
// testdata/api_vocabulary.txt (E-122, ZYS-1190 freeze) honest against the
// live root-package surface: the guard fails when a live exported
// identifier is missing from the file, when the file classes a live
// identifier Removed, and when the file lists a non-Removed name that
// nothing in the live surface has.
func TestVocabularyGuard_FlagsUnclassifiedRemovedOrUnexportedNames(t *testing.T) {
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	names := vocabulary.NamesFromSurface(live)

	entries, err := vocabulary.ParseFile("testdata/api_vocabulary.txt")
	if err != nil {
		t.Fatal(err)
	}

	if v := vocabulary.Check(names, entries); !v.Empty() {
		t.Fatalf("exported vocabulary drifted from testdata/api_vocabulary.txt (see docs/philosophy/domain-vocabulary.md and the vocabulary freeze contract):\n%s", v)
	}
}

// TestVocabularyGuardCheck_DetectsEachDisagreementKind unit-tests
// vocabulary.Check directly against small fixtures, independent of the
// live surface, so each of the three failure modes has its own assertion.
func TestVocabularyGuardCheck_DetectsEachDisagreementKind(t *testing.T) {
	entries := []vocabulary.Entry{
		{Name: "Keep", Class: vocabulary.Canonical, Concept: "Keep"},
		{Name: "Gone", Class: vocabulary.Removed, Concept: "Keep"},
		{Name: "Ghost", Class: vocabulary.Helper, Concept: "Keep"},
	}

	v := vocabulary.Check([]string{"Keep", "Gone", "New"}, entries)

	if got := v.MissingFromFile; len(got) != 1 || got[0] != "New" {
		t.Fatalf("MissingFromFile: want [New], got %v", got)
	}
	if got := v.ClassedRemoved; len(got) != 1 || got[0] != "Gone" {
		t.Fatalf("ClassedRemoved: want [Gone], got %v", got)
	}
	if got := v.UnexportedListed; len(got) != 1 || got[0] != "Ghost" {
		t.Fatalf("UnexportedListed: want [Ghost], got %v", got)
	}
	if v.Empty() {
		t.Fatal("Violations with entries must not report Empty")
	}
}

// TestVocabularyParse_RejectsClassOutsideClosedSet proves Parse enforces
// the same closed canonical/helper/removed set the header documents,
// instead of accepting any class string and letting Check treat an
// unrecognized class as live (a mutation of testdata/api_vocabulary.txt
// classing a real entry a made-up class must fail loudly, not pass
// silently as though the entry were canonical).
func TestVocabularyParse_RejectsClassOutsideClosedSet(t *testing.T) {
	_, err := vocabulary.Parse("Ghost\tbogus\tKeep\n")
	if err == nil {
		t.Fatal("Parse accepted a class outside canonical/helper/removed")
	}
}
