// Package main is doc.go's package quickstart verbatim (see
// TestPackageDocQuickstartMatchesFixture). TestPackageDocQuickstartRuns
// runs it and fails if any row is left unresolved.
package main

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

// docexamples:snippet start
func main() {
	evo.Init(evo.Config{Title: "repo"}) // first statement — arms first paint before any I/O
	os.Exit(evo.Main(run))
}

func run(ctx context.Context) error {
	evo.Println("Reading configuration")
	evo.Task("working tree").Define(checkWorkingTree)
	status := evo.Task("git status")
	status.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "git", "status", "--short")
		cmd.Stdout = status.Writer() // the child's output is this row's evidence
		cmd.Stderr = status.Writer()
		return cmd.Run()
	})
	return nil
}

// docexamples:snippet end

// checkWorkingTree is the one name the quickstart leaves to the reader.
func checkWorkingTree(ctx context.Context) error { return nil }
