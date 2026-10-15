// legacy8: a v1.0.0 program (gate 8 review 1) using surfaces the earlier
// legacy probes did not: evo.Run, ExecSpec Basis with FSPath/Value, package
// Confirm with PolicyFlag, Task.Fail with NextCommand, Update + Affected,
// Bytes, Step, Key, package Fact/Warn and Verbose().Println.
package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "sync assets", DryRun: len(os.Args) > 1})
	res := evo.Run(context.Background(), run)
	os.Exit(res.Conclusion.ExitCode)
}

func run(ctx context.Context) error {
	evo.Fact("target", "s3://assets")
	evo.Verbose().Println("resolving manifest")
	gen := evo.Task("generate").Key("gen")
	gen.Define(func(ctx context.Context) error {
		return evo.Exec(ctx, evo.ExecSpec{
			Executable: "make",
			Args:       []string{"assets"},
			Basis:      []evo.Fingerprint{evo.FSPath("assets.yaml"), evo.Value("level", 3)},
			Outputs:    []string{"dist/assets.tar"},
		})
	})
	upload := evo.Task("upload").After(gen)
	upload.Bytes(0, 4096)
	upload.Step(1, 3, "manifest")
	if !evo.Confirm("upload 3 objects?", evo.PolicyFlag("--yes")) {
		upload.Fail("upload declined", evo.NextCommand("sync", "--yes"))
		return nil
	}
	upload.Update("bucket object", func() error { return push(ctx) }, evo.Affected(3))
	evo.Warn("cache headers not set")
	return nil
}

func push(ctx context.Context) error {
	if _, err := fmt.Fprintln(os.Stderr, "pushing", ctx != nil); err != nil {
		return fmt.Errorf("announce push: %w", err)
	}
	return nil
}
