package stagename_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/stagename"
)

func TestNewIsAStagingNameBesideTheDestination(t *testing.T) {
	dest := filepath.Join("some", "dir", "report.txt")
	got := stagename.New(dest)
	if filepath.Dir(got) != filepath.Dir(dest) {
		t.Fatalf("New(%q) = %q, want a sibling of the destination", dest, got)
	}
	if name := filepath.Base(got); !stagename.Is(name) {
		t.Fatalf("New(%q) = %q, whose name is not the staging shape", dest, got)
	}
}

func TestNewNamesAreUnpredictable(t *testing.T) {
	dest := filepath.Join("dir", "report.txt")
	if first, second := stagename.New(dest), stagename.New(dest); first == second {
		t.Fatalf("New returned %q twice", first)
	}
}

func TestNewNameCarriesTheOwnersPrefix(t *testing.T) {
	dest := filepath.Join("dir", "report.txt")
	if name := filepath.Base(stagename.New(dest)); !strings.HasPrefix(name, stagename.Owner(dest)) {
		t.Fatalf("name %q lacks the owner prefix %q", name, stagename.Owner(dest))
	}
}

func TestOwnerBindsToTheDestinationsBaseNameOnly(t *testing.T) {
	a := stagename.Owner(filepath.Join("one", "report.txt"))
	b := stagename.Owner(filepath.Join("two", "report.txt"))
	other := stagename.Owner(filepath.Join("one", "other.txt"))
	if a != b {
		t.Fatalf("the same base name in two directories has owners %q and %q", a, b)
	}
	if a == other {
		t.Fatalf("different base names share the owner %q", a)
	}
}

func TestIsAcceptsOnlyTheExactShape(t *testing.T) {
	good := filepath.Base(stagename.New("report.txt"))
	owner := stagename.Owner("report.txt")
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
		if got := stagename.Is(tc.in); got != tc.want {
			t.Errorf("%s: Is(%q) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}
