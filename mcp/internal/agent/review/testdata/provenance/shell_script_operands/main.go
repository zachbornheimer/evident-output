package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{Name: "probe demo"}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Summary("tidy the demo workspace").Fact("mode", "probe")
	gen := r.Group("generate")
	for _, n := range []string{"a", "b", "c"} {
		gen.Task("write " + n).Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: n + ".txt", Contents: []byte(n + "\n"), Mode: 0o644, Inputs: []string{"seed.txt"}})
		})
	}
	seq := r.Sequence("build")
	inv := evo.Compute(seq.Task("discover"), func(ctx context.Context) ([]string, error) { return []string{"x", "y", "keep"}, nil })
	seq.Group("prune").After(inv).Build(func(g *evo.Group) {
		for _, b := range inv.Get() {
			g.Task("prune " + b).Define(func(ctx context.Context) error {
				if b == "keep" {
					return evo.Exclude("protected")
				}
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 1, Resource: "branch/" + b}, func(context.Context) error { return nil })
			})
		}
	})
	r.Task("concat").After(gen).Define(func(ctx context.Context) error {
		res, execErr := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"sh", "-c", "cat a.txt b.txt c.txt > all.txt"}, Inputs: []string{"a.txt", "b.txt", "c.txt"}, Outputs: []string{"all.txt"}})
		_ = res
		return execErr
	}).Problem("demo note", evo.On("all.txt"))
	if os.Getenv("ASK") != "" && !r.Confirm("really finish?", evo.Destructive(), evo.NonInteractive("--yes")) {
		return evo.Refuse("declined")
	}
	fmt.Fprintln(r.ResultWriter(), "done ok")
	return nil
}
