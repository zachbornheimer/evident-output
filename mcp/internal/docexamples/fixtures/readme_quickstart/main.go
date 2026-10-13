// docexamples:snippet start
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() { os.Exit(evo.Main(run)) } // Main wires SIGINT/SIGTERM and returns the exit code

func run(ctx context.Context, r *evo.Run) error {
	r.Task("check config").Define(checkConfig)

	installs := r.Group("install") // children may overlap
	for _, pkg := range packages {
		installs.Task(pkg).Define(func(ctx context.Context) error { return install(pkg) })
	}

	r.Sequence("publish").After(installs).Build(func(s *evo.Sequence) {
		s.Task("generate").Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: "out.txt", Contents: []byte("ok\n"), Mode: 0o644})
		})
		s.Task("prune").Define(func(ctx context.Context) error {
			spec := evo.EffectSpec{Verb: evo.Delete, Object: "stale branch", Quantity: 2}
			return evo.Effect(ctx, spec, removeStaleBranches) // ledger: "deleted 2 stale branches"
		})
	})
	return nil
}

// docexamples:snippet end
