package change

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fsKey(path string) ResourceKey          { return ResourceKey{space: spaceFS, name: path} }
func logicalKey(name string) ResourceKey     { return ResourceKey{space: spaceLogical, name: name} }
func claim(k ResourceKey, m ClaimMode) Claim { return Claim{Key: k, Mode: m} }
func readOf(k ResourceKey) Claim             { return claim(k, ClaimRead) }
func writeOf(k ResourceKey) Claim            { return claim(k, ClaimWrite) }
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(TempDir): %v", err)
	}
	return dir
}

func TestKeyOverlap(t *testing.T) {
	cases := []struct {
		name string
		a, b ResourceKey
		want bool
	}{
		{"same fs path", fsKey("/repo/a.go"), fsKey("/repo/a.go"), true},
		{"fs ancestor", fsKey("/repo"), fsKey("/repo/pkg/a.go"), true},
		{"fs descendant", fsKey("/repo/pkg/a.go"), fsKey("/repo"), true},
		{"fs root is everyone's ancestor", fsKey("/"), fsKey("/repo/a.go"), true},
		{"fs siblings", fsKey("/repo/a.go"), fsKey("/repo/b.go"), false},
		{"fs shared name prefix is not ancestry", fsKey("/repo/a"), fsKey("/repo/ab"), false},
		{"same logical name", logicalKey("brew"), logicalKey("brew"), true},
		{"different logical names", logicalKey("brew"), logicalKey("brew/cask"), false},
		{"fs and logical never overlap", fsKey("/brew"), logicalKey("/brew"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.overlaps(tc.b); got != tc.want {
				t.Fatalf("%v overlaps %v = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestClaimConflict(t *testing.T) {
	k := fsKey("/repo")
	child := fsKey("/repo/a.go")
	cases := []struct {
		name string
		a, b Claim
		want bool
	}{
		{"read/read overlap", readOf(k), readOf(child), false},
		{"read/write conflict", readOf(k), writeOf(child), true},
		{"write/read conflict", writeOf(k), readOf(child), true},
		{"write/write conflict", writeOf(child), writeOf(child), true},
		{"write/write disjoint", writeOf(fsKey("/x")), writeOf(fsKey("/y")), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.conflicts(tc.b); got != tc.want {
				t.Fatalf("%v conflicts %v = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestResolveFSRelativeJoinsWorkspace(t *testing.T) {
	ws := realTempDir(t)
	got, err := ResolveResource(FSResource("pkg/../a.go"), ws)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := fsKey(filepath.Join(ws, "a.go")); got != want {
		t.Fatalf("Resolve = %v, want %v", got, want)
	}
}

func TestResolveFSAbsoluteIgnoresWorkspace(t *testing.T) {
	ws := realTempDir(t)
	got, err := ResolveResource(FSResource(filepath.Join(ws, "x", "..", "a.go")), "/elsewhere")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := fsKey(filepath.Join(ws, "a.go")); got != want {
		t.Fatalf("Resolve = %v, want %v", got, want)
	}
}

// A symlinked directory is an alias: claims through it must land on the
// same canonical key as claims through the real path, or the alias would
// bypass coordination.
func TestResolveFSSymlinkAliasSharesIdentity(t *testing.T) {
	ws := realTempDir(t)
	real := filepath.Join(ws, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(ws, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	viaAlias, err := ResolveResource(FSResource("alias/missing/new.go"), ws)
	if err != nil {
		t.Fatalf("Resolve alias: %v", err)
	}
	viaReal, err := ResolveResource(FSResource(filepath.Join(real, "missing", "new.go")), ws)
	if err != nil {
		t.Fatalf("Resolve real: %v", err)
	}
	if viaAlias != viaReal {
		t.Fatalf("alias key %v != real key %v", viaAlias, viaReal)
	}
	dirViaAlias, err := ResolveResource(FSResource("alias"), ws)
	if err != nil {
		t.Fatalf("Resolve alias dir: %v", err)
	}
	if !dirViaAlias.overlaps(viaReal) {
		t.Fatalf("alias dir %v must overlap child %v", dirViaAlias, viaReal)
	}
}

func TestResolveFSPropagatesNonMissingErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	failing := resourceResolver{evalSymlinks: func(string) (string, error) { return "", boom }}
	if _, err := failing.resolve(FSResource("/a"), "/"); !errors.Is(err, boom) {
		t.Fatalf("Resolve error = %v, want wrapping %v", err, boom)
	}
}

func TestResolveRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		r    Resource
		ws   string
	}{
		{"empty fs path", FSResource(""), "/ws"},
		{"relative fs path without absolute workspace", FSResource("a.go"), "."},
		{"empty logical name", LogicalResource("  \t"), "/ws"},
		{"nil resource", nil, "/ws"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ResolveResource(tc.r, tc.ws); !errors.Is(err, ErrInvalidResource) {
				t.Fatalf("Resolve error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestResolveLogicalTrimsSurroundingSpace(t *testing.T) {
	got, err := ResolveResource(LogicalResource("  homebrew "), "/ws")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := logicalKey("homebrew"); got != want {
		t.Fatalf("Resolve = %v, want %v", got, want)
	}
}

func TestResourceString(t *testing.T) {
	if got := FSResource("go.mod").(fmtStringer).String(); got != "fs:go.mod" {
		t.Fatalf("FS String = %q", got)
	}
	if got := LogicalResource("homebrew").(fmtStringer).String(); got != "logical:homebrew" {
		t.Fatalf("Logical String = %q", got)
	}
}

type fmtStringer interface{ String() string }

func TestLabelIsTheCallersName(t *testing.T) {
	for r, want := range map[Resource]string{
		FSResource("./go.mod"):         "./go.mod",
		LogicalResource("  homebrew "): "homebrew",
	} {
		if got := ResourceLabel(r); got != want {
			t.Errorf("Label(%v) = %q, want %q", r, got, want)
		}
	}
}
