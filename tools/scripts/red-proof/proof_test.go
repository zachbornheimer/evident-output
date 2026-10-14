package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeGit struct {
	exportErr error
	changed   []string // files the fix commit changed
	missing   bool     // the repository lacks the fix commit
}

func (g fakeGit) Export(string, string) error { return g.exportErr }

func (g fakeGit) ChangedFiles(string) ([]string, bool, error) {
	return g.changed, !g.missing, nil
}

type fakeSandbox struct {
	applyErr error
	out      map[string][]byte
	applied  *[]byte // receives the patch ApplyPatch was given
}

func (s fakeSandbox) ApplyPatch(_ string, patch []byte) error {
	if s.applied != nil {
		*s.applied = patch
	}
	return s.applyErr
}

func (s fakeSandbox) RunTests(_, pkg string, _ []string) ([]byte, error) {
	return s.out[pkg], nil
}

func fakePatch(name string) ([]byte, error) {
	switch name {
	case "patches/missing.patch":
		return nil, errors.New("no such patch")
	case "patches/stray.patch":
		return []byte("--- a/x.go\n+++ b/x.go\n--- a/other.go\n+++ b/other.go\n"), nil
	}
	return []byte("patch " + name), nil
}

func fakeScratch() (string, func(), error) { return "scratch", func() {}, nil }

func proverWith(git Git, sb Sandbox) Prover {
	return Prover{Git: git, Sandbox: sb, Scratch: fakeScratch, ReadPatch: fakePatch}
}

func redGreen(red, green string) []byte {
	return events(
		`{"Action":"run","Test":"`+red+`"}`, `{"Action":"fail","Test":"`+red+`"}`,
		`{"Action":"run","Test":"`+green+`"}`, `{"Action":"pass","Test":"`+green+`"}`,
	)
}

var entry = Entry{Commit: "abcdef1", Patch: "patches/abcdef1.patch", Tests: []TestRef{{".", "TestA"}, {".", "TestB"}}}

func TestProveIsProvenOnlyWhenEveryTestIsRed(t *testing.T) {
	allRed := events(
		`{"Action":"run","Test":"TestA"}`, `{"Action":"fail","Test":"TestA"}`,
		`{"Action":"run","Test":"TestB"}`, `{"Action":"fail","Test":"TestB"}`,
	)
	res := proverWith(fakeGit{}, fakeSandbox{out: map[string][]byte{".": allRed}}).Prove(entry)
	if res.Status != Proven {
		t.Fatalf("status = %s, want PASS; lines %v", res.Status, res.Lines)
	}
}

func TestProveNamesATestThatStaysGreen(t *testing.T) {
	res := proverWith(fakeGit{}, fakeSandbox{out: map[string][]byte{".": redGreen("TestA", "TestB")}}).Prove(entry)
	if res.Status != StayedGreen {
		t.Fatalf("status = %s, want GREEN", res.Status)
	}
	if !strings.Contains(strings.Join(res.Lines, "\n"), "green   . TestB") {
		t.Errorf("lines do not name TestB as green: %v", res.Lines)
	}
}

func TestProveIsInconclusiveWhenATestDidNotRun(t *testing.T) {
	build := events(`{"Action":"build-output","Output":"undefined: Plan\n"}`, `{"Action":"build-fail"}`)
	res := proverWith(fakeGit{}, fakeSandbox{out: map[string][]byte{".": build}}).Prove(entry)
	if res.Status != Inconclusive {
		t.Fatalf("status = %s, want INCONCLUSIVE", res.Status)
	}
	if !strings.Contains(strings.Join(res.Lines, "\n"), "undefined: Plan") {
		t.Errorf("lines omit the build failure: %v", res.Lines)
	}
}

func TestProveReportsSetupFailuresAsErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]Prover{
		"export": proverWith(fakeGit{exportErr: boom}, fakeSandbox{}),
		"apply":  proverWith(fakeGit{}, fakeSandbox{applyErr: boom}),
	}
	for name, p := range cases {
		if res := p.Prove(entry); res.Status != Errored || !strings.Contains(res.Lines[0], "boom") {
			t.Errorf("%s: status %s lines %v, want ERROR naming boom", name, res.Status, res.Lines)
		}
	}
}

func TestProveAppliesTheEntrysPatch(t *testing.T) {
	allRed := events(
		`{"Action":"run","Test":"TestA"}`, `{"Action":"fail","Test":"TestA"}`,
		`{"Action":"run","Test":"TestB"}`, `{"Action":"fail","Test":"TestB"}`,
	)
	applied := []byte(nil)
	res := proverWith(fakeGit{}, fakeSandbox{applied: &applied, out: map[string][]byte{".": allRed}}).Prove(entry)
	if res.Status != Proven {
		t.Fatalf("status = %s, want PASS; lines %v", res.Status, res.Lines)
	}
	if string(applied) != "patch patches/abcdef1.patch" {
		t.Errorf("patch applied = %q, want the entry's patch", applied)
	}
	if !strings.Contains(res.Basis, "patches/abcdef1.patch") {
		t.Errorf("basis = %q, want the patch named", res.Basis)
	}
}

func TestProveErrorsWhenThePatchIsUnusable(t *testing.T) {
	cases := map[string]struct {
		patch string
		sb    fakeSandbox
	}{
		"absent":       {"", fakeSandbox{}},
		"missing":      {"patches/missing.patch", fakeSandbox{}},
		"does not fit": {"patches/abcdef1.patch", fakeSandbox{applyErr: errors.New("patch does not apply")}},
	}
	for name, c := range cases {
		e := entry
		e.Patch = c.patch
		if res := proverWith(fakeGit{}, c.sb).Prove(e); res.Status != Errored {
			t.Errorf("%s: status = %s, want ERROR, never PASS; lines %v", name, res.Status, res.Lines)
		}
	}
}

func TestProveRefusesAPatchBeyondTheFixCommitsFiles(t *testing.T) {
	e := entry
	e.Patch = "patches/stray.patch"
	res := proverWith(fakeGit{changed: []string{"x.go"}}, fakeSandbox{}).Prove(e)
	if res.Status != Errored || !strings.Contains(strings.Join(res.Lines, "\n"), "other.go") {
		t.Fatalf("status = %s, lines %v; want ERROR naming other.go", res.Status, res.Lines)
	}
}

func TestProveSaysWhenTheFixCommitIsMissingSoScopeIsUnchecked(t *testing.T) {
	e := entry
	e.Patch = "patches/stray.patch"
	allRed := events(
		`{"Action":"run","Test":"TestA"}`, `{"Action":"fail","Test":"TestA"}`,
		`{"Action":"run","Test":"TestB"}`, `{"Action":"fail","Test":"TestB"}`,
	)
	res := proverWith(fakeGit{missing: true}, fakeSandbox{out: map[string][]byte{".": allRed}}).Prove(e)
	if res.Status != Proven || !strings.Contains(strings.Join(res.Lines, "\n"), "scope unchecked") {
		t.Fatalf("status = %s, lines %v; want PASS with a scope-unchecked note", res.Status, res.Lines)
	}
}
