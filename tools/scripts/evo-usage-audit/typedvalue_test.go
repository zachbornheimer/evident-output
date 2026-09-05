package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestTypedValueCrossFileFieldClassifies covers work order done-when 1(a):
// Report (file_b.go) reaches evo only through Reporter.out, a field
// declared on a struct in a SEPARATE same-package file (file_a.go), with no
// evo. qualifier of its own — it must classify as evo-typed value.
func TestTypedValueCrossFileFieldClassifies(t *testing.T) {
	dir := copyFixture(t, filepath.Join("testdata", "typedvalue"))
	decl := findDecl(t, dir, "func (r Reporter) Report()")
	if decl.Usage != usageEvoTypedValue {
		t.Fatalf("Report() usage = %v, want %v", decl.Usage, usageEvoTypedValue)
	}
}

// TestTypedValueWrongOwnerFieldNotClassified covers work order done-when
// 1(b): Widget also has a field named "out", but it's a plain string —
// Emit must NOT classify, proving detection keys on the owner's type, not
// the bare field name.
func TestTypedValueWrongOwnerFieldNotClassified(t *testing.T) {
	dir := copyFixture(t, filepath.Join("testdata", "typedvalue"))
	assertDeclAbsent(t, dir, "func (w Widget) Emit()")
}

// TestTypedValueLocalVarNotClassified covers work order done-when 1(c): a
// local variable literally named "out", calling a same-shaped Fact(...)
// method, must NOT classify — classifyTypedValue never resolves a local
// var's type, so an unresolved base is left unclassified rather than
// guessed from the name matching Reporter.out.
func TestTypedValueLocalVarNotClassified(t *testing.T) {
	dir := copyFixture(t, filepath.Join("testdata", "typedvalue"))
	assertDeclAbsent(t, dir, "func UsesLocalOut()")
}

// findDecl scans dir and returns the one declFinding whose source contains
// signature, failing the test if it's missing (rather than classifying a
// declaration that was never found as "absent").
func findDecl(t *testing.T, dir, signature string) declFinding {
	t.Helper()
	files, err := scanRepo(dir)
	if err != nil {
		t.Fatalf("scanRepo(%s): %v", dir, err)
	}
	for _, f := range files {
		for _, d := range f.Decls {
			if strings.Contains(d.Source, signature) {
				return d
			}
		}
	}
	t.Fatalf("no classified decl found containing %q in %s", signature, dir)
	return declFinding{}
}

// assertDeclAbsent proves signature never appears among dir's classified
// declarations at all — the correct shape for "not classified", since an
// unclassified declaration is omitted from scanRepo's output entirely
// (see scanDir), not present with usageNone.
func assertDeclAbsent(t *testing.T, dir, signature string) {
	t.Helper()
	files, err := scanRepo(dir)
	if err != nil {
		t.Fatalf("scanRepo(%s): %v", dir, err)
	}
	for _, f := range files {
		for _, d := range f.Decls {
			if strings.Contains(d.Source, signature) {
				t.Fatalf("%q must not be classified, got usage %v", signature, d.Usage)
			}
		}
	}
}
