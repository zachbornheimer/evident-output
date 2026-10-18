// Fixture: a contract-legal program whose Effect deletes a file and claims
// it with EffectSpec.Path. API-057 must stay silent: evo.File cannot express
// a deletion.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/v2"
)

func main() {
	os.Exit(evo.Main(evo.Config{Name: "probe7 ws", DryRun: os.Getenv("DRY") != ""}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	r.Summary("render, install, clean, bundle").Fact("seed", "seed.txt")
	notes := r.Group("render notes")
	for _, n := range []string{"alpha", "beta"} {
		notes.Task(n).Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: n + ".md", Contents: []byte(n + "\n"), Mode: 0o644, Inputs: []string{"seed.txt"}})
		}).Fact("words", "1")
	}
	r.Task("install hook").Define(func(ctx context.Context) error {
		if err := evo.File(ctx, evo.FileSpec{Path: "hook.sh", Contents: []byte("#!/bin/sh\n"), Mode: 0o755}); err != nil {
			return evo.Fail("hook not installed", evo.On("hook.sh"), evo.Detail(err.Error()))
		}
		return nil
	}).Problem("hooks are optional", evo.On("hooks/"))
	clean := r.Group("clean caches")
	for _, m := range []string{"npm", "pip"} {
		mg := clean.Group(m)
		for _, f := range []string{"a", "b"} {
			p := filepath.Join("cache", m, f)
			mg.Task(f).Define(func(ctx context.Context) error {
				if _, err := os.Stat(p); err != nil {
					return nil
				}
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "cache entry", Quantity: 1, Path: p}, func(context.Context) error { return os.Remove(p) })
			})
		}
	}
	ship := r.Sequence("ship")
	list := evo.Compute(ship.Task("list").After(notes), func(context.Context) ([]string, error) { return []string{"alpha.md", "beta.md"}, nil })
	ship.Task("bundle").Define(func(ctx context.Context) error {
		if _, err := evo.Exec(ctx, evo.ExecSpec{Argv: append([]string{"tar", "cf", "notes.tar"}, list.Get()...), Inputs: list.Get(), Outputs: []string{"notes.tar"}}); err != nil {
			return fmt.Errorf("bundle notes.tar: %w", err)
		}
		return nil
	})
	if os.Getenv("ASK") != "" && !r.Confirm("publish notes.tar?", evo.NonInteractive("--yes")) {
		return evo.Refuse("declined")
	}
	fmt.Fprintln(r.ResultWriter(), "notes.tar")
	return nil
}
