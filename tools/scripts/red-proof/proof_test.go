package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeGit struct {
	exportErr, diffErr error
	diffs              *int // counts FixChanges calls
}

func (g fakeGit) Export(string, string) error { return g.exportErr }
func (g fakeGit) FixChanges(string, string) ([]FileChange, error) {
	if g.diffs != nil {
		*g.diffs++
	}
	return nil, g.diffErr
}

type fakeSandbox struct {
	undoErr, applyErr error
	out               map[string][]byte
	applied           *[]byte // receives the patch ApplyRevert was given
}

func (s fakeSandbox) UndoFix(string, []FileChange) error { return s.undoErr }

func (s fakeSandbox) ApplyRevert(_ string, patch []byte) error {
	if s.applied != nil {
		*s.applied = patch
	}
	return s.applyErr
}

func (s fakeSandbox) RunTests(_, pkg string, _ []string) ([]byte, error) {
	return s.out[pkg], nil
}

func fakePatch(name string) ([]byte, error) {
	if name == "patches/missing.patch" {
		return nil, errors.New("no such patch")
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

var entry = Entry{Commit: "abcdef1", Tests: []TestRef{{".", "TestA"}, {".", "TestB"}}}

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
		"diff":   proverWith(fakeGit{diffErr: boom}, fakeSandbox{}),
		"undo":   proverWith(fakeGit{}, fakeSandbox{undoErr: boom}),
	}
	for name, p := range cases {
		if res := p.Prove(entry); res.Status != Errored || !strings.Contains(res.Lines[0], "boom") {
			t.Errorf("%s: status %s lines %v, want ERROR naming boom", name, res.Status, res.Lines)
		}
	}
}

func TestMovedPathsNamesRenamesAndCopies(t *testing.T) {
	got := movedPaths("M\ta.go\nR087\told.go\tnew.go\nC100\tsrc.go\tcopy.go\n")
	if strings.Join(got, "|") != "old.go -> new.go|src.go -> copy.go" {
		t.Errorf("movedPaths = %v", got)
	}
}

func TestProveErrorsWhenTheFixCannotBeReverted(t *testing.T) {
	conflict := &ConflictError{Path: "report.go", Reason: "2 conflicting hunks"}
	allRed := events(
		`{"Action":"run","Test":"TestA"}`, `{"Action":"fail","Test":"TestA"}`,
		`{"Action":"run","Test":"TestB"}`, `{"Action":"fail","Test":"TestB"}`,
	)
	res := proverWith(fakeGit{}, fakeSandbox{undoErr: conflict, out: map[string][]byte{".": allRed}}).Prove(entry)
	if res.Status != Errored {
		t.Fatalf("status = %s, want ERROR, never PASS; lines %v", res.Status, res.Lines)
	}
	if !strings.Contains(strings.Join(res.Lines, "\n"), "report.go") {
		t.Errorf("lines do not name the conflicting path: %v", res.Lines)
	}
}

func TestProveAppliesTheHandWrittenRevertInPlaceOfTheCommit(t *testing.T) {
	allRed := events(
		`{"Action":"run","Test":"TestA"}`, `{"Action":"fail","Test":"TestA"}`,
		`{"Action":"run","Test":"TestB"}`, `{"Action":"fail","Test":"TestB"}`,
	)
	diffs, applied := 0, []byte(nil)
	reverted := entry
	reverted.Revert = "patches/abcdef1.patch"
	// A commit-reversal conflict must not matter: the patch replaces that path.
	sb := fakeSandbox{undoErr: &ConflictError{Path: "report.go"}, applied: &applied, out: map[string][]byte{".": allRed}}
	res := proverWith(fakeGit{diffs: &diffs}, sb).Prove(reverted)
	if res.Status != Proven {
		t.Fatalf("status = %s, want PASS; lines %v", res.Status, res.Lines)
	}
	if diffs != 0 || string(applied) != "patch patches/abcdef1.patch" {
		t.Errorf("commit read %d times, patch applied %q; want the patch and no commit reversal", diffs, applied)
	}
	if !strings.Contains(res.Basis, "patches/abcdef1.patch") {
		t.Errorf("basis = %q, want the patch named", res.Basis)
	}
}

func TestProveErrorsWhenTheRevertPatchIsUnusable(t *testing.T) {
	cases := map[string]struct {
		revert string
		sb     fakeSandbox
	}{
		"missing":      {"patches/missing.patch", fakeSandbox{}},
		"does not fit": {"patches/abcdef1.patch", fakeSandbox{applyErr: errors.New("patch does not apply")}},
	}
	for name, c := range cases {
		e := entry
		e.Revert = c.revert
		if res := proverWith(fakeGit{}, c.sb).Prove(e); res.Status != Errored {
			t.Errorf("%s: status = %s, want ERROR; lines %v", name, res.Status, res.Lines)
		}
	}
}
