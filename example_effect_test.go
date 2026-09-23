package evo_test

import (
	"context"
	"fmt"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleEffect records an opaque mutation Evo cannot model as file state —
// here, removing stale worktrees. Under DryRun the callback never runs and
// the ledger shows the planned Effect; without DryRun the callback runs and
// the Effect is recorded as changed only after it returns nil.
func ExampleEffect() {
	for _, dryRun := range []bool{true, false} {
		var buf strings.Builder
		out := evo.Init(evo.Config{Isolated: true, Title: "prune", Plain: true, Color: evo.ColorNever, DryRun: dryRun, Stdout: &buf})
		removed := 0
		out.Task("worktrees").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectRemove, Object: "stale worktree", Quantity: 2},
				func(ctx context.Context) error {
					removed = 2 // e.g. `git worktree remove` for each stale path, honoring ctx
					return nil
				})
		})
		_ = out.Finish()
		snap := out.Snapshot()
		fmt.Printf("dryRun=%v removed=%d planned=%d changed=%d\n", dryRun, removed, len(snap.Plans), len(snap.Changes))
	}
	// Output:
	// dryRun=true removed=0 planned=1 changed=0
	// dryRun=false removed=2 planned=0 changed=1
}

// ExampleEffectSpec describes one aggregate opaque mutation: Object is the
// singular noun and Quantity the positive count the ledger pluralizes from.
// Constructing it performs no I/O — passing it to Effect does.
func ExampleEffectSpec() {
	spec := evo.EffectSpec{Verb: evo.EffectDelete, Object: "remote ref", Quantity: 3}
	fmt.Println(spec.Verb, spec.Quantity, spec.Object)
	// Output:
	// delete 3 remote ref
}

// ExampleEffectVerb lists the closed verb set. There is no write verb:
// file state goes through File, not an opaque Effect.
func ExampleEffectVerb() {
	for _, v := range []evo.EffectVerb{evo.EffectAdd, evo.EffectCreate, evo.EffectDelete, evo.EffectPush, evo.EffectRemove, evo.EffectUpdate} {
		fmt.Print(v, " ")
	}
	fmt.Println()
	// Output:
	// add create delete push remove update
}

// ExampleEffectSpec_noResource shows the default: an EffectSpec with no
// Resource makes no resource claim. Resource is sealed — callers cannot
// implement it.
func ExampleEffectSpec_noResource() {
	spec := evo.EffectSpec{Verb: evo.EffectPush, Object: "branch", Quantity: 1}
	fmt.Println(spec.Resource == nil)
	// Output:
	// true
}
