// Fixture: legacy_run_program.go with every suggestion applied, and the
// package-level Warn placed on the upload Task by hand, the one step review
// can only name.
package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	res := evo.Execute(context.Background(), evo.Config{Name: "sync assets", DryRun: len(os.Args) > 1}, run)
	os.Exit(res.ExitCode())
}

func run(ctx context.Context, r *evo.Run) error {
	r.Fact("target", "s3://assets")
	fmt.Fprintln(r.Text(), "resolving manifest")
	gen := r.Task("generate")
	gen.Define(func(ctx context.Context) error {
		if _, err := evo.Exec(ctx, evo.ExecSpec{
			Argv:    []string{"make", "assets"},
			Inputs:  []string{"assets.yaml"},
			Values:  map[string]string{"level": "3"},
			Outputs: []string{"dist/assets.tar"},
		}); err != nil {
			return err
		}
		return nil
	})
	upload := r.Task("upload").After(gen)
	upload.Progress(0, 4096)
	upload.Progress(1, 3).Doing("manifest")
	if !r.Confirm("upload 3 objects?", evo.NonInteractive("--yes")) {
		upload.Define(func(ctx context.Context) error {
			return evo.Fail("upload declined", evo.Next(evo.Action{Command: []string{"sync", "--yes"}}))
		})
		return nil
	}
	upload.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, Object: "bucket object", Quantity: 3}, func(context.Context) error { return push(ctx) })
	})
	upload.Problem("cache headers not set")
	return nil
}

func push(ctx context.Context) error {
	if _, err := fmt.Fprintln(os.Stderr, "pushing", ctx != nil); err != nil {
		return fmt.Errorf("announce push: %w", err)
	}
	return nil
}
