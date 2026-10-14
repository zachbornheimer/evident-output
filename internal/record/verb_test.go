package record

import "testing"

// TestPastTense_CompoundVerbs is the red-first table for hyphenated
// mutation verbs (evo-rec Problem 2, the remote-tracking Problem): the
// spec's [changed] rows spell delete-remote's past tense "deleted" and
// fetch-prune's past tense "pruned", and an unmapped compound falls back
// to conjugating its first segment and dropping the rest.
func TestPastTense_CompoundVerbs(t *testing.T) {
	cases := []struct {
		verb string
		want string
	}{
		{"delete-remote", "deleted"},
		{"fetch-prune", "pruned"},
		{"sync-remote", "synced"}, // unmapped compound: general fallback rule
	}
	for _, tc := range cases {
		if got := PastTense(tc.verb); got != tc.want {
			t.Errorf("PastTense(%q) = %q, want %q", tc.verb, got, tc.want)
		}
	}
}

// TestPastTense_SingleWordVerbsUnaffected pins that the compound-verb
// fallback does not disturb existing single-word conjugation (irregulars
// and the default +d/+ed rule).
func TestPastTense_SingleWordVerbsUnaffected(t *testing.T) {
	cases := []struct {
		verb string
		want string
	}{
		{"delete", "deleted"},
		{"push", "pushed"},
		{"write", "wrote"},
	}
	for _, tc := range cases {
		if got := PastTense(tc.verb); got != tc.want {
			t.Errorf("PastTense(%q) = %q, want %q", tc.verb, got, tc.want)
		}
	}
}
