package evo

import "testing"

// TestConjugatePast_CompoundVerbs is the red-first table for hyphenated
// mutation verbs (evo-rec Problem 2, the remote-tracking Problem): the
// spec's [changed] rows spell delete-remote's past tense "deleted" and
// fetch-prune's past tense "pruned", and an unmapped compound falls back
// to conjugating its first segment and dropping the rest.
func TestConjugatePast_CompoundVerbs(t *testing.T) {
	cases := []struct {
		verb string
		want string
	}{
		{"delete-remote", "deleted"},
		{"fetch-prune", "pruned"},
		{"sync-remote", "synced"}, // unmapped compound: general fallback rule
	}
	for _, tc := range cases {
		if got := conjugatePast(tc.verb); got != tc.want {
			t.Errorf("conjugatePast(%q) = %q, want %q", tc.verb, got, tc.want)
		}
	}
}

// TestConjugatePast_SingleWordVerbsUnaffected pins that the compound-verb
// fallback does not disturb existing single-word conjugation (irregulars
// and the default +d/+ed rule).
func TestConjugatePast_SingleWordVerbsUnaffected(t *testing.T) {
	cases := []struct {
		verb string
		want string
	}{
		{"delete", "deleted"},
		{"push", "pushed"},
		{"write", "wrote"},
	}
	for _, tc := range cases {
		if got := conjugatePast(tc.verb); got != tc.want {
			t.Errorf("conjugatePast(%q) = %q, want %q", tc.verb, got, tc.want)
		}
	}
}

// TestPluralize_RegularAndIrregularNouns pins L6: Pluralize is the object
// counterpart to conjugatePast's verb table, so Delete/Update/Remove/Push
// call sites stop writing their own singular/plural noun() switch.
func TestPluralize_RegularAndIrregularNouns(t *testing.T) {
	cases := []struct {
		quantity int64
		singular string
		want     string
	}{
		{1, "worktree", "worktree"},
		{2, "worktree", "worktrees"},
		{0, "worktree", "worktrees"},
		{2, "branch", "branches"},
		{2, "box", "boxes"},
		{2, "buzz", "buzzes"},
		{2, "batch", "batches"},
		{2, "dish", "dishes"},
		{2, "policy", "policies"},
		{2, "day", "days"}, // vowel+y keeps regular +s
		{2, "child", "children"},
		{1, "child", "child"},
		{3, "index", "indices"},
	}
	for _, tc := range cases {
		if got := Pluralize(tc.quantity, tc.singular); got != tc.want {
			t.Errorf("Pluralize(%d, %q) = %q, want %q", tc.quantity, tc.singular, got, tc.want)
		}
	}
}
