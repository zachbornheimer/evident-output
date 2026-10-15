// Fixture: EVO-DRYRUN-001 must stay silent. The os.RemoveAll in the prune task
// runs inside an evo.Effect callback whose EffectSpec.Path claims the file,
// the contract's own opaque-mutation path; dry run never invokes it.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{Name: "probe6 ws"}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Summary("render notes, bundle, prune").Fact("seed", "seed.txt")
	notes := r.Group("render notes")
	for _, n := range []string{"alpha", "beta", "gamma"} {
		notes.Task(n).Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: n + ".md", Contents: []byte(strings.ToUpper(n) + "\n"), Inputs: []string{"seed.txt"}})
		}).Fact("words", "1")
	}
	ship := r.Sequence("ship")
	files := evo.Compute(ship.Task("bundle").After(notes), func(ctx context.Context) ([]string, error) {
		_, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"tar", "cf", "notes.tar", "alpha.md", "beta.md", "gamma.md"}, Inputs: []string{"alpha.md", "beta.md", "gamma.md"}, Outputs: []string{"notes.tar"}})
		return []string{"alpha.md", "beta.md", "keep.md"}, err
	})
	ship.Group("prune").Build(func(g *evo.Group) {
		for _, f := range files.Get() {
			g.Task("prune " + f).Define(func(ctx context.Context) error {
				if f == "keep.md" {
					return evo.Exclude("protected")
				}
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "stale copy", Quantity: 1, Path: "old/" + f}, func(context.Context) error { return os.RemoveAll("old/" + f) })
			})
		}
	})
	r.Task("audit").After(ship).Verify(func(context.Context) (bool, error) { _, err := os.Stat("audit.ok"); return err == nil, nil }).Define(func(ctx context.Context) error {
		fmt.Fprintln(r.Text(), "audit note")
		return evo.File(ctx, evo.FileSpec{Path: "audit.ok"})
	}).Problem("audit is advisory", evo.On("audit.ok"), evo.Code("advisory"))
	if os.Getenv("ASK") != "" && !r.Confirm("remove old copies?", evo.Destructive(), evo.NonInteractive("--yes")) {
		return evo.Refuse("declined")
	}
	fmt.Fprintln(r.ResultWriter(), "bundle notes.tar")
	return nil
}
