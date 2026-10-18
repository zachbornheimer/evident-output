package main

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "prune", DryRun: len(os.Args) > 1, Verbosity: evo.VerbosityVerbose, Format: evo.FormatJSON})
	os.Exit(evo.Main(run))
}

func run(ctx context.Context) error {
	evo.Fact("remote", "origin")
	if !evo.Confirm("delete branches?", evo.AssumeYes(false), evo.PolicyFlag("--yes")) {
		return nil
	}
	branches := evo.Group("branches")
	for _, b := range []string{"a", "b"} {
		b := b
		branches.Task(b).Define(func(ctx context.Context) error {
			spec := evo.EffectSpec{Verb: evo.EffectDelete, Object: "branches", Quantity: 1}
			return evo.Effect(ctx, spec, func(ctx context.Context) error { return exec.Command("git", "branch", "-D", b).Run() })
		})
	}
	lint := evo.Task("lint").Doing("scanning")
	lint.Warn("slow")
	lint.Define(func(ctx context.Context) error {
		cmd := exec.Command("golangci-lint", "run")
		cmd.Stdout = lint.Writer()
		if err := cmd.Run(); err != nil {
			return evo.Fail("lint failed: %v", err)
		}
		return nil
	})
	if err := lint.Wait(); err != nil {
		return err
	}
	evo.Task("tags").Define(func(ctx context.Context) error { return evo.Skipped(evo.Reason("protected")) })
	evo.Print("done\n")
	return nil
}
