package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUsageInventoryGoldenFixture pins renderMarkdown's output against
// testdata/fixture, a fixture covering every classification rule: a
// direct-use func, an evo-typed-signature func with a method receiver, a
// type decl with an evo-typed field, a grouped var(...) block with one
// qualifying spec, and an irrelevant func that must not appear at all.
//
// The fixture is copied into a fresh t.TempDir() before scanning — never
// scanned in place — so this repo's own git ancestry can never leak a live
// branch/SHA into the golden, keeping the comparison deterministic. Run
// with UPDATE_GOLDEN=1 to regenerate after a deliberate rendering change.
func TestUsageInventoryGoldenFixture(t *testing.T) {
	dir := copyFixture(t, filepath.Join("testdata", "fixture"))

	files, err := scanRepo(dir)
	if err != nil {
		t.Fatalf("scanRepo: %v", err)
	}
	meta := gatherRepoMeta(dir)
	if meta.HasBranchAndSHA {
		t.Fatalf("fixture temp copy must not resolve as a git worktree, got branch %q sha %q", meta.Branch, meta.SHA)
	}

	got := renderMarkdown(dir, files, meta)
	normalized := strings.ReplaceAll(got, dir, "TESTDATA_ROOT")

	goldenPath := filepath.Join("testdata", "fixture.golden.md")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if normalized != string(want) {
		t.Errorf("renderMarkdown(testdata/fixture) drifted from golden.\ngot:\n%s\nwant:\n%s\n"+
			"(re-run with UPDATE_GOLDEN=1 if this drift is deliberate)", normalized, want)
	}
}

// TestUsageInventoryOmitsIrrelevantDecl names the one fixture site the
// golden must never contain, so a future golden-format refactor can't
// silently drop this coverage the way a golden-only test would.
func TestUsageInventoryOmitsIrrelevantDecl(t *testing.T) {
	dir := copyFixture(t, filepath.Join("testdata", "fixture"))
	files, err := scanRepo(dir)
	if err != nil {
		t.Fatalf("scanRepo: %v", err)
	}
	for _, f := range files {
		for _, d := range f.Decls {
			if strings.Contains(d.Source, "func Irrelevant()") {
				t.Fatalf("Irrelevant must be omitted (touches no evo identifier), got it classified %s", d.Usage)
			}
		}
	}
}

// copyFixture copies src's regular files into a fresh temp directory and
// returns that directory's path.
func copyFixture(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read fixture dir %s: %v", src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatalf("read fixture file %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), data, 0o644); err != nil {
			t.Fatalf("write fixture copy %s: %v", entry.Name(), err)
		}
	}
	return dst
}
