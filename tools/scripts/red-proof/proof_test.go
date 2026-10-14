package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeGit struct {
	exportErr, diffErr error
}

func (g fakeGit) Export(string, string) error                     { return g.exportErr }
func (g fakeGit) FixChanges(string, string) ([]FileChange, error) { return nil, g.diffErr }

type fakeSandbox struct {
	undoErr    error
	undoErrors []error // consumed one per UndoFix call, before undoErr
	out        map[string][]byte
	calls      *int
}

func (s fakeSandbox) UndoFix(string, []FileChange) error {
	if s.calls != nil {
		defer func() { *s.calls++ }()
		if *s.calls < len(s.undoErrors) {
			return s.undoErrors[*s.calls]
		}
	}
	return s.undoErr
}

func (s fakeSandbox) RunTests(_, pkg string, _ []string) ([]byte, error) {
	return s.out[pkg], nil
}

func fakeScratch() (string, func(), error) { return "scratch", func() {}, nil }

func proverWith(git Git, sb Sandbox) Prover {
	return Prover{Git: git, Sandbox: sb, Scratch: fakeScratch}
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

func TestProveFallsBackToTheFixCommitWhenHeadCannotBeReverted(t *testing.T) {
	allRed := events(
		`{"Action":"run","Test":"TestA"}`, `{"Action":"fail","Test":"TestA"}`,
		`{"Action":"run","Test":"TestB"}`, `{"Action":"fail","Test":"TestB"}`,
	)
	calls := 0
	conflict := &ConflictError{Path: "report.go", Reason: "2 conflicting hunks"}
	sb := fakeSandbox{undoErrors: []error{conflict}, out: map[string][]byte{".": allRed}, calls: &calls}
	res := proverWith(fakeGit{}, sb).Prove(entry)
	if res.Status != Proven {
		t.Fatalf("status = %s, want PASS; lines %v", res.Status, res.Lines)
	}
	if !strings.Contains(res.Basis, "abcdef1") || !strings.Contains(res.Basis, "report.go") {
		t.Errorf("basis = %q, want the fix commit and the conflicting path named", res.Basis)
	}
}

func TestProveDoesNotHideAConflictThatPersistsAtTheFixCommit(t *testing.T) {
	conflict := &ConflictError{Path: "report.go", Reason: "x"}
	res := proverWith(fakeGit{}, fakeSandbox{undoErr: conflict}).Prove(entry)
	if res.Status != Errored {
		t.Fatalf("status = %s, want ERROR", res.Status)
	}
}
