// Fixture: legacy_nested_groups.go with every suggestion applied verbatim.
package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	os.Exit(evo.Main(evo.Config{Name: "prune caches"}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	caches := r.Group("caches")
	for _, m := range []string{"npm", "pip"} {
		mg := caches.Group(m)
		for _, f := range []string{"a", "b"} {
			t := mg.Task(f)
			if f == "b" {
				t.Define(func(ctx context.Context) error { return evo.Exclude("pinned") })
				continue
			}
			t.Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "cache entry", Quantity: 1}, func(context.Context) error { return os.Remove(m + "/" + f) })
			})
		}
	}
	build := r.Task("build").After(caches)
	build.Define(func(ctx context.Context) error {
		if err := compile(ctx); err != nil {
			return evo.Fail(fmt.Sprintf("build failed: %v", err))
		}
		build.Problem("vet skipped")
		return nil
	})
	check := r.Task("check remote")
	if os.Getenv("REMOTE") == "" {
		check.Define(func(ctx context.Context) error { return evo.Refuse(fmt.Sprintf("no remote %s", "origin")) })
	} else {
		check.Define(func(ctx context.Context) error { return nil })
	}
	return nil
}

func compile(context.Context) error { return nil }
