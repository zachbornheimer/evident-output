// Fixture: a 1.x program with nested Groups, Delete, Skipped/Reason, Warn,
// Failf, Blockf and Done. Every rewrite review offers is applied, and the
// result may leave only the findings that came with advice, not a rewrite.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "prune caches"})
	os.Exit(evo.Main(run))
}

func run(ctx context.Context) error {
	caches := evo.Group("caches")
	for _, m := range []string{"npm", "pip"} {
		mg := caches.Group(m)
		for _, f := range []string{"a", "b"} {
			t := mg.Task(f)
			if f == "b" {
				t.Skipped(evo.Reason("pinned"))
				continue
			}
			t.Delete("cache entry", func() error { return os.Remove(m + "/" + f) })
		}
	}
	build := evo.Task("build").After(caches)
	build.Define(func(ctx context.Context) error {
		if err := compile(ctx); err != nil {
			return build.Failf("build failed: %v", err)
		}
		build.Warn("vet skipped")
		return nil
	})
	check := evo.Task("check remote")
	if os.Getenv("REMOTE") == "" {
		check.Blockf("no remote %s", "origin")
	} else {
		check.Done()
	}
	return nil
}

func compile(context.Context) error { return nil }
