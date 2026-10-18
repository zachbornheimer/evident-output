// Fixture: bad_deletion.go with EVO-DRYRUN-001's suggestion applied. A
// deletion inside an Effect that claims its Path is the contract's own
// shape; no rule may ask for it to become evo.File.
package main

import (
	"context"
	"os"

	"github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("drop stale lock").Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "file", Quantity: 1, Path: "app.lock"}, func(context.Context) error {
			return os.Remove("app.lock")
		})
	})
	return nil
}
