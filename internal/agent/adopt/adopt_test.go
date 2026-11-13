package adopt_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/adopt"
)

// TestInventoryMixedFixtureGolden pins adopt.Inventory's output against
// testdata/mixed, a fixture that mixes fmt.Printf, log.Println, log.Fatal,
// and a manual spinner import — the exact shape a real pre-adoption CLI
// carries. Run with -update to regenerate the golden after a deliberate
// detection change.
func TestInventoryMixedFixtureGolden(t *testing.T) {
	got, err := adopt.Inventory(filepath.Join("testdata", "mixed"))
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	// Golden compares on file basename only — the fixture's absolute path
	// depends on where the module checkout lives.
	for i := range got.Findings {
		got.Findings[i].File = filepath.Base(got.Findings[i].File)
	}
	got.Directory = filepath.Base(got.Directory)

	goldenPath := filepath.Join("testdata", "mixed.golden.json")
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotJSON = append(gotJSON, '\n')

	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, gotJSON, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(gotJSON) != string(want) {
		t.Errorf("Inventory(testdata/mixed) drifted from golden.\ngot:\n%s\nwant:\n%s\n(re-run with UPDATE_GOLDEN=1 if this drift is deliberate)", gotJSON, want)
	}
}

// TestInventoryFlagsEveryFixtureSite is the non-golden half: it names each
// site the golden pins so a future refactor of the golden format cannot
// silently drop coverage of one.
func TestInventoryFlagsEveryFixtureSite(t *testing.T) {
	plan, err := adopt.Inventory(filepath.Join("testdata", "mixed"))
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	wantPatterns := map[string]bool{
		"import github.com/briandowns/spinner": false,
		"fmt.Printf":                           false,
		"log.Println":                          false,
		"log.Fatal":                            false,
	}
	for _, f := range plan.Findings {
		if _, ok := wantPatterns[f.Pattern]; ok {
			wantPatterns[f.Pattern] = true
		}
		if f.Suggestion == "" {
			t.Errorf("finding %+v has no suggestion — adopt must always say why or say it can't", f)
		}
	}
	for pattern, found := range wantPatterns {
		if !found {
			t.Errorf("Inventory missed expected pattern %q: %+v", pattern, plan.Findings)
		}
	}
}

// TestInventoryUnknownDirectory proves Inventory reports an honest error
// rather than an empty, silently-successful plan.
func TestInventoryUnknownDirectory(t *testing.T) {
	if _, err := adopt.Inventory(filepath.Join("testdata", "does-not-exist")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

// TestInventoryDetectsOutputFacade proves adopt reports a wrapped-logger
// type (testdata/facade, modeled on go-task's internal/logger) as its own
// facade finding — migrate the type, not each call site — rather than
// missing it the way a per-call-site-only classifier does. It was RED
// before facade detection existed: the fixture's Outf/Errf/Warnf call
// sites are invisible to fmt/os/log call-site classification alone.
func TestInventoryDetectsOutputFacade(t *testing.T) {
	plan, err := adopt.Inventory(filepath.Join("testdata", "facade"))
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(plan.Facades) != 1 {
		t.Fatalf("want exactly 1 facade, got %d: %+v", len(plan.Facades), plan.Facades)
	}

	got := plan.Facades[0]
	if got.Type != "Logger" {
		t.Errorf("Type = %q, want %q", got.Type, "Logger")
	}
	wantMethods := []string{"Errf", "Outf", "Warnf"}
	if len(got.Methods) != len(wantMethods) {
		t.Fatalf("Methods = %v, want %v", got.Methods, wantMethods)
	}
	for i, m := range wantMethods {
		if got.Methods[i] != m {
			t.Errorf("Methods[%d] = %q, want %q", i, got.Methods[i], m)
		}
	}

	// main.go calls Outf twice, Errf once, Warnf once: 4 real call sites.
	if len(got.CallSites) != 4 {
		t.Errorf("CallSites = %v, want 4 entries", got.CallSites)
	}
	if got.Note == "" {
		t.Error("facade finding has no migrate-the-facade note")
	}

	if plan.Caveat == "" {
		t.Error("plan with a detected facade must disclose that inventory is a floor, not a census")
	}
}

// TestInventorySkipsTestFiles proves *_test.go is not an adoption finding —
// tests print; migrating them is not the adoption unit of work.
func TestInventorySkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	prod := "package p\nimport \"fmt\"\nfunc F() { fmt.Println(\"prod\") }\n"
	testSrc := "package p\nimport \"fmt\"\nfunc TestF() { fmt.Println(\"test\") }\n"
	if err := os.WriteFile(filepath.Join(dir, "prod.go"), []byte(prod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prod_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := adopt.Inventory(dir)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	for _, f := range plan.Findings {
		if strings.HasSuffix(f.File, "_test.go") {
			t.Errorf("*_test.go must not be an adopt finding: %+v", f)
		}
	}
	if len(plan.Findings) != 1 {
		t.Fatalf("want 1 production finding, got %d: %+v", len(plan.Findings), plan.Findings)
	}
	if plan.Findings[0].Pattern != "fmt.Println" {
		t.Errorf("production finding pattern = %q, want fmt.Println", plan.Findings[0].Pattern)
	}
}

// TestInventoryDoesNotTreatDefaultsLogfAsFacade proves a Config.defaults
// closure that fmt.Fprintf(os.Stderr) is not a facade — the type has no
// io.Writer fields. testdata/facade (writer fields + wrap-through) stays
// a facade; this fixture is the false-positive that used to match.
func TestInventoryDoesNotTreatDefaultsLogfAsFacade(t *testing.T) {
	plan, err := adopt.Inventory(filepath.Join("testdata", "defaults"))
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(plan.Facades) != 0 {
		t.Fatalf("defaults() Logf assignment is not a facade, got %d: %+v", len(plan.Facades), plan.Facades)
	}
}

// TestInventoryPrefersCmdSubtree proves that when dir/cmd exists, inventory
// walks that subtree instead of every Go file under dir.
func TestInventoryPrefersCmdSubtree(t *testing.T) {
	dir := t.TempDir()
	rootSrc := "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"root\") }\n"
	cmdDir := filepath.Join(dir, "cmd", "app")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmdSrc := "package main\nimport \"log\"\nfunc main() { log.Fatal(\"cmd\") }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(rootSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(cmdSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := adopt.Inventory(dir)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(plan.Findings) != 1 {
		t.Fatalf("want 1 cmd/ finding, got %d: %+v", len(plan.Findings), plan.Findings)
	}
	if plan.Findings[0].Pattern != "log.Fatal" {
		t.Errorf("pattern = %q, want log.Fatal (cmd/), not the root fmt.Println", plan.Findings[0].Pattern)
	}
}
