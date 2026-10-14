// Command red-proof replays the proof that each ledgered fix is guarded: for
// every entry in ledger.yaml it reverts the fix's non-test hunks in a scratch
// copy of HEAD and requires every named test to fail there.
//
//	go run ./tools/scripts/red-proof [-ledger FILE] [-repo DIR] [-only COMMIT]
//
// Exit 0: every test is red without its fix. Exit 1: a test stayed green or
// did not run (it does not guard the fix). Exit 2: a replay could not be set up.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	exitProven       = 0
	exitUnguarded    = 1
	exitReplayFailed = 2
)

func main() {
	var ledgerPath, repo, only string
	flag.StringVar(&ledgerPath, "ledger", "tools/scripts/red-proof/ledger.yaml", "ledger of fix commits and guarding tests")
	flag.StringVar(&repo, "repo", ".", "repository root")
	flag.StringVar(&only, "only", "", "replay only the entry whose commit starts with this")
	flag.Parse()
	patchDir := filepath.Dir(ledgerPath)
	prover := Prover{
		Git: repoGit{root: repo}, Sandbox: scratchSandbox{}, Scratch: tempScratch,
		ReadPatch: func(name string) ([]byte, error) { return readFile(filepath.Join(patchDir, name)) },
	}
	os.Exit(execute(ledgerPath, only, prover, os.Stdout))
}

func execute(ledgerPath, only string, prover Prover, out io.Writer) int {
	text, err := readFile(ledgerPath)
	if err != nil {
		_, _ = fmt.Fprintf(out, "read ledger: %v\n", err)
		return exitReplayFailed
	}
	ledger, err := ParseLedger(string(text))
	if err != nil {
		_, _ = fmt.Fprintf(out, "parse %s: %v\n", ledgerPath, err)
		return exitReplayFailed
	}
	code := exitProven
	replayed := 0
	for _, entry := range ledger.Entries {
		if only != "" && !strings.HasPrefix(entry.Commit, only) {
			continue
		}
		replayed++
		res := prover.Prove(entry)
		report(out, res)
		code = max(code, exitCode(res.Status))
	}
	if replayed == 0 {
		_, _ = fmt.Fprintf(out, "no ledger entry matches -only %q\n", only)
		return exitReplayFailed
	}
	return code
}

func exitCode(s Status) int {
	switch s {
	case Proven:
		return exitProven
	case Errored:
		return exitReplayFailed
	}
	return exitUnguarded
}

func report(out io.Writer, res Result) {
	_, _ = fmt.Fprintf(out, "%s %s: %s\n    fix removed in: %s\n", res.Status, res.Entry.Commit, res.Entry.Note, res.Basis)
	for _, line := range res.Lines {
		_, _ = fmt.Fprintf(out, "    %s\n", line)
	}
	switch res.Status {
	case StayedGreen:
		_, _ = fmt.Fprintf(out, "    %s: a test stays green without the fix, so it does not guard it\n", res.Entry.Commit)
	case Inconclusive:
		_, _ = fmt.Fprintf(out, "    %s: a test did not run or was skipped, so nothing is proven\n", res.Entry.Commit)
	}
}
