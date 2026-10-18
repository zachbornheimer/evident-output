package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	names := []string{"a", "b"}
	r.Task("bound").Define(func(ctx context.Context) error {
		return evo.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))
	})
	r.Task("generate").Define(func(ctx context.Context) error {
		if _, err := evo.Exec(ctx, evo.ExecSpec{
			Argv:    []string{"python3", "generate.py", "input.xlsx", "template.txt", "build/out.bin"},
			Inputs:  []string{"input.xlsx"},
			Outputs: []string{"build/out.bin"},
		}); err != nil {
			return fmt.Errorf("generate build/out.bin: %w", err)
		}
		return nil
	})
	r.Task("install").Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Verb("install"), Object: "packages", Quantity: 2}, install)
	})
	return nil
}

func install(context.Context) error { return os.WriteFile("x", nil, 0o600) }
