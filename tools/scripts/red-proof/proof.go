package main

import (
	"fmt"
	"strings"
)

// Git is the repository boundary; tests substitute a fake.
type Git interface {
	// Export writes the tree at rev into an empty directory.
	Export(rev, dir string) error
}

// Sandbox is the scratch-tree boundary; tests substitute a fake.
type Sandbox interface {
	// ApplyPatch applies a hand-written patch that re-introduces the bug in
	// dir. A patch that does not apply is an error.
	ApplyPatch(dir string, patch []byte) error
	// RunTests runs the named tests of one package in dir and returns the
	// `go test -json` output, whether or not the tests passed.
	RunTests(dir, pkg string, names []string) ([]byte, error)
}

// ScratchDir makes an empty directory and returns the function that removes it.
type ScratchDir func() (dir string, remove func(), err error)

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
	// Basis names the tree the bug was re-introduced in.
	Basis string
	Lines []string
}

const headRev = "HEAD"

// Prover replays ledger entries against scratch copies of HEAD.
type Prover struct {
	Git     Git
	Sandbox Sandbox
	Scratch ScratchDir
	// ReadPatch returns the hand-written patch an entry names.
	ReadPatch func(name string) ([]byte, error)
}

// Prove re-introduces the bug into a scratch copy of HEAD by the entry's
// hand-written patch and runs the entry's tests; the entry is Proven only when
// every test fails there. A patch that is missing or does not apply is an
// error, never a pass.
func (p Prover) Prove(e Entry) Result {
	res := Result{Entry: e, Basis: headRev + " with " + e.Patch}
	fail := func(err error) Result {
		res.Status = Errored
		res.Lines = append(res.Lines, err.Error())
		return res
	}
	if e.Patch == "" {
		return fail(fmt.Errorf("entry %s names no patch", e.Commit))
	}
	dir, remove, err := p.Scratch()
	if err != nil {
		return fail(fmt.Errorf("make scratch directory: %w", err))
	}
	defer remove()
	if err := p.Git.Export(headRev, dir); err != nil {
		return fail(fmt.Errorf("export %s: %w", headRev, err))
	}
	if err := p.applyPatch(dir, e); err != nil {
		return fail(err)
	}
	return p.replay(dir, res)
}

func (p Prover) applyPatch(dir string, e Entry) error {
	patch, err := p.ReadPatch(e.Patch)
	if err != nil {
		return fmt.Errorf("read patch %s: %w", e.Patch, err)
	}
	if err := p.Sandbox.ApplyPatch(dir, patch); err != nil {
		return fmt.Errorf("apply patch %s for fix %s: %w", e.Patch, e.Commit, err)
	}
	return nil
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
