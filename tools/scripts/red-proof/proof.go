package main

import (
	"errors"
	"fmt"
	"strings"
)

// Git is the repository boundary; tests substitute a fake.
type Git interface {
	// Export writes the tree at rev into an empty directory.
	Export(rev, dir string) error
	// FixChanges returns the commit's changes to Go source other than
	// *_test.go files, each with its blobs, spelled for the tree at rev. It
	// fails on a renamed or copied file, naming the path.
	FixChanges(commit, rev string) ([]FileChange, error)
}

// ChangeKind is how a fix commit touched one file.
type ChangeKind string

const (
	// Added means the commit created the file.
	Added ChangeKind = "A"
	// Modified means the commit edited the file.
	Modified ChangeKind = "M"
	// Deleted means the commit removed the file.
	Deleted ChangeKind = "D"
)

// FileChange is one non-test Go file a fix commit touched. Fixed is the file
// as the commit left it (nil if Deleted); Before is the file as the commit
// found it (nil if Added).
type FileChange struct {
	Path   string
	Kind   ChangeKind
	Fixed  []byte
	Before []byte
}

// Sandbox is the scratch-tree boundary; tests substitute a fake.
type Sandbox interface {
	// UndoFix reverts changes inside dir by a three-way merge, so edits made
	// after the fix commit survive. A change that cannot be reverted cleanly
	// yields a *ConflictError.
	UndoFix(dir string, changes []FileChange) error
	// RunTests runs the named tests of one package in dir and returns the
	// `go test -json` output, whether or not the tests passed.
	RunTests(dir, pkg string, names []string) ([]byte, error)
}

// ScratchDir makes an empty directory and returns the function that removes it.
type ScratchDir func() (dir string, remove func(), err error)

// ConflictError reports a fix that later edits keep from being reverted.
type ConflictError struct {
	Path   string
	Reason string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("reverting the fix conflicts with later edits in %s: %s", e.Path, e.Reason)
}

// Status is the outcome of replaying one ledger entry.
type Status string

const (
	// Proven means every named test failed without the fix.
	Proven Status = "PASS"
	// StayedGreen means a test passed without the fix, so it does not guard it.
	StayedGreen Status = "GREEN"
	// Inconclusive means a test did not run or was skipped.
	Inconclusive Status = "INCONCLUSIVE"
	// Errored means the replay itself could not be set up.
	Errored Status = "ERROR"
)

// Result is one entry's status with the lines that explain it.
type Result struct {
	Entry  Entry
	Status Status
	// Basis names the tree the fix was reverted in.
	Basis string
	Lines []string
}

const headRev = "HEAD"

// Prover replays ledger entries against scratch copies of HEAD.
type Prover struct {
	Git     Git
	Sandbox Sandbox
	Scratch ScratchDir
}

// Prove reverts the fix's non-test hunks in a scratch copy of HEAD and runs
// the entry's tests; the entry is Proven only when every test fails there.
// When later edits make the revert conflict, it falls back to the fix commit's
// own tree, where the revert is exact, and says so in the result's Basis.
func (p Prover) Prove(e Entry) Result {
	res := p.proveAt(e, headRev)
	var conflict *ConflictError
	if !errors.As(res.err, &conflict) {
		return res.Result
	}
	fallback := p.proveAt(e, e.Commit)
	fallback.Basis = fmt.Sprintf("fix commit %s (HEAD revert conflicts in %s)", e.Commit, conflict.Path)
	return fallback.Result
}

type attempt struct {
	Result
	err error
}

func (p Prover) proveAt(e Entry, rev string) attempt {
	res := attempt{Result: Result{Entry: e, Basis: rev}}
	fail := func(err error) attempt {
		res.Status, res.err = Errored, err
		res.Lines = append(res.Lines, err.Error())
		return res
	}
	dir, remove, err := p.Scratch()
	if err != nil {
		return fail(fmt.Errorf("make scratch directory: %w", err))
	}
	defer remove()
	if err := p.Git.Export(rev, dir); err != nil {
		return fail(fmt.Errorf("export %s: %w", rev, err))
	}
	changes, err := p.Git.FixChanges(e.Commit, rev)
	if err != nil {
		return fail(fmt.Errorf("read fix %s: %w", e.Commit, err))
	}
	if err := p.Sandbox.UndoFix(dir, changes); err != nil {
		return fail(fmt.Errorf("revert fix %s: %w", e.Commit, err))
	}
	res.Result = p.replay(dir, res.Result)
	return res
}

func (p Prover) replay(dir string, res Result) Result {
	pkgs, names := res.Entry.PackageTests()
	res.Status = Proven
	for _, pkg := range pkgs {
		out, err := p.Sandbox.RunTests(dir, pkg, names[pkg])
		if err != nil {
			res.Status = Errored
			res.Lines = append(res.Lines, fmt.Sprintf("run %s: %v", pkg, err))
			return res
		}
		verdicts := Verdicts(out, names[pkg])
		for _, name := range names[pkg] {
			res.Lines = append(res.Lines, fmt.Sprintf("%-7s %s %s", verdicts[name], pkg, name))
			res.Status = worse(res.Status, verdicts[name])
		}
		if res.Status == Inconclusive {
			res.Lines = append(res.Lines, "package output: "+strings.ReplaceAll(PackageOutput(out), "\n", "\n    "))
		}
	}
	return res
}

func worse(current Status, v Verdict) Status {
	switch {
	case current == StayedGreen || v == Green:
		return StayedGreen
	case current == Inconclusive || v == Skipped || v == NotRun:
		return Inconclusive
	}
	return current
}
