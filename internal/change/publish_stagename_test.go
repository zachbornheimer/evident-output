package change

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStagingNameIsAStagingNameBesideTheDestination(t *testing.T) {
	dest := filepath.Join("some", "dir", "report.txt")
	got := stagingName(dest)
	if filepath.Dir(got) != filepath.Dir(dest) {
		t.Fatalf("stagingName(%q) = %q, want a sibling of the destination", dest, got)
	}
	if name := filepath.Base(got); !IsStaging(name) {
		t.Fatalf("stagingName(%q) = %q, whose name is not the staging shape", dest, got)
	}
}

func TestStagingNamesAreUnpredictable(t *testing.T) {
	dest := filepath.Join("dir", "report.txt")
	if first, second := stagingName(dest), stagingName(dest); first == second {
		t.Fatalf("stagingName returned %q twice", first)
	}
}

func TestStagingNameCarriesTheOwnersPrefix(t *testing.T) {
	dest := filepath.Join("dir", "report.txt")
	if name := filepath.Base(stagingName(dest)); !strings.HasPrefix(name, stagingOwner(dest)) {
		t.Fatalf("name %q lacks the owner prefix %q", name, stagingOwner(dest))
	}
}

func TestOwnerBindsToTheDestinationsBaseNameOnly(t *testing.T) {
	a := stagingOwner(filepath.Join("one", "report.txt"))
	b := stagingOwner(filepath.Join("two", "report.txt"))
	other := stagingOwner(filepath.Join("one", "other.txt"))
	if a != b {
		t.Fatalf("the same base name in two directories has owners %q and %q", a, b)
	}
	if a == other {
		t.Fatalf("different base names share the owner %q", a)
	}
}

func TestIsStagingAcceptsOnlyTheExactShape(t *testing.T) {
	good := filepath.Base(stagingName("report.txt"))
	owner := stagingOwner("report.txt")
	random := strings.TrimSuffix(strings.TrimPrefix(good, owner), ".tmp")
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"generated name", good, true},
		{"empty", "", false},
		{"ordinary file", "report.txt", false},
		{"missing prefix", strings.TrimPrefix(good, "."), false},
		{"missing suffix", strings.TrimSuffix(good, ".tmp"), false},
		{"uppercase random part", owner + strings.ToUpper(random) + ".tmp", false},
		{"non-hex owner", ".evo-zzzzzzzz-" + random + ".tmp", false},
		{"short random part", owner + random[1:] + ".tmp", false},
		{"long random part", owner + random + "a.tmp", false},
		{"digit outside the alphabet", owner + "1" + random[1:] + ".tmp", false},
		{"no separator", ".evo-" + strings.Repeat("a", 8) + "x" + random + ".tmp", false},
	}
	for _, tc := range cases {
		if got := IsStaging(tc.in); got != tc.want {
			t.Errorf("%s: IsStaging(%q) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}
