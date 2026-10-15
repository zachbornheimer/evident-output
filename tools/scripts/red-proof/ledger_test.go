package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const moduleRoot = "../../.."

// roundFixes are the round-1/2 concurrency fixes the gate-3 review could not
// replay; the ledger must carry every one.
var roundFixes = []string{"ddbd70f3", "d4965643", "852ea4b8", "8c9f44b6", "f0d90673"}

func TestLedgerNamesEveryRoundFix(t *testing.T) {
	text, err := readFile("ledger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := ParseLedger(string(text))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ledger.Entries {
		got = append(got, e.Commit)
		for _, test := range e.Tests {
			if !testExists(t, test) {
				t.Errorf("entry %s: no func %s(*testing.T) in a *_test.go file of %s", e.Commit, test.Name, test.Pkg)
			}
		}
	}
	for _, want := range roundFixes {
		if !slices.Contains(got, want) {
			t.Errorf("ledger does not name round fix %s", want)
		}
	}
}

func testExists(t *testing.T, ref TestRef) bool {
	t.Helper()
	dir := filepath.Join(moduleRoot, filepath.FromSlash(ref.Pkg))
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files in %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == ref.Name {
				return true
			}
		}
	}
	return false
}

// Every entry replays by a hand-written patch, so each must name one that
// exists, or the replay would have nothing to apply.
func TestLedgerRevertPatchesExist(t *testing.T) {
	text, err := readFile("ledger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := ParseLedger(string(text))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ledger.Entries {
		if e.Patch == "" {
			t.Errorf("entry %s names no patch", e.Commit)
			continue
		}
		if _, err := readFile(e.Patch); err != nil {
			t.Errorf("entry %s: %v", e.Commit, err)
		}
	}
}

// A patch re-introduces the fix commit's bug; one that reaches a file the fix
// never touched is no longer that bug. The commit object may be missing from a
// bare archive, and then the check says so rather than passing silently.
func TestLedgerPatchTouchesOnlyTheFixCommitsFiles(t *testing.T) {
	text, err := readFile("ledger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := ParseLedger(string(text))
	if err != nil {
		t.Fatal(err)
	}
	repo := repoGit{root: moduleRoot}
	for _, e := range ledger.Entries {
		t.Run(e.Commit, func(t *testing.T) {
			fixFiles, available, err := repo.ChangedFiles(e.Commit)
			if err != nil {
				t.Fatal(err)
			}
			if !available {
				t.Skipf("commit %s is not in this repository (bare archive), so its files cannot be listed", e.Commit)
			}
			patch, err := readFile(e.Patch)
			if err != nil {
				t.Fatal(err)
			}
			patched := patchPaths(patch)
			if len(patched) == 0 {
				t.Fatalf("%s patches no file", e.Patch)
			}
			if outside := outsideFix(patched, fixFiles); len(outside) > 0 {
				t.Errorf("%s patches %v, which commit %s did not change (or are test files)", e.Patch, outside, e.Commit)
			}
		})
	}
}

func TestParseLedgerReadsEntriesAndTests(t *testing.T) {
	text := "entries:\n  - commit: abcdef1\n    note: \"a: b\"\n    patch: patches/a.patch\n    tests:\n      - pkg: ./x\n        name: TestOne\n      - pkg: .\n        name: TestTwo\n"
	ledger, err := ParseLedger(text)
	if err != nil {
		t.Fatal(err)
	}
	want := Entry{Commit: "abcdef1", Note: "a: b", Patch: "patches/a.patch", Tests: []TestRef{{"./x", "TestOne"}, {".", "TestTwo"}}}
	if len(ledger.Entries) != 1 || !slices.Equal(ledger.Entries[0].Tests, want.Tests) || ledger.Entries[0].Note != want.Note {
		t.Fatalf("entries = %+v, want [%+v]", ledger.Entries, want)
	}
}

func TestParseLedgerRejectsWhatItCannotReplay(t *testing.T) {
	const head = "entries:\n  - commit: abcdef1\n    patch: patches/a.patch\n    tests:\n"
	cases := map[string]string{
		"unknown key":      head + "      - pkg: .\n        name: TestA\n    owner: me\n",
		"no tests":         "entries:\n  - commit: abcdef1\n    patch: patches/a.patch\n",
		"no patch":         "entries:\n  - commit: abcdef1\n    tests:\n      - pkg: .\n        name: TestA\n",
		"bad commit":       "entries:\n  - commit: NOPE\n    patch: patches/a.patch\n    tests:\n      - pkg: .\n        name: TestA\n",
		"escaping package": head + "      - pkg: ../x\n        name: TestA\n",
		"non-test name":    head + "      - pkg: .\n        name: helper\n",
		"missing name":     head + "      - pkg: .\n",
		"duplicate commit": head + "      - pkg: .\n        name: TestA\n  - commit: abcdef1\n    patch: patches/a.patch\n    tests:\n      - pkg: .\n        name: TestA\n",
		"stray line":       "entries:\nnonsense\n",
		"patch outside":    "entries:\n  - commit: abcdef1\n    patch: ../x.patch\n    tests:\n      - pkg: .\n        name: TestA\n",
	}
	for name, text := range cases {
		if _, err := ParseLedger(text); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestEntryGroupsTestsByPackageInFirstSeenOrder(t *testing.T) {
	e := Entry{Tests: []TestRef{{"./b", "TestB1"}, {".", "TestR"}, {"./b", "TestB2"}}}
	pkgs, names := e.PackageTests()
	if got := strings.Join(pkgs, ","); got != "./b,." {
		t.Errorf("packages = %s, want ./b,.", got)
	}
	if !slices.Equal(names["./b"], []string{"TestB1", "TestB2"}) {
		t.Errorf("./b tests = %v", names["./b"])
	}
}
