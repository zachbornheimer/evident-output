package evo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// ExampleEffect records an opaque mutation Evo cannot model as file state —
// here, removing stale worktrees. Under DryRun the callback never runs and
// the ledger shows the planned Effect; without DryRun the callback runs and
// the Effect is recorded as changed only after it returns nil.
func ExampleEffect() {
	for _, dryRun := range []bool{true, false} {
		var buf strings.Builder
		out := initRun(Config{Name: "prune", DryRun: dryRun, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
		removed := 0
		out.Task("worktrees").Define(func(ctx context.Context) error {
			return Effect(ctx, EffectSpec{Verb: Verb(change.EffectRemove), Object: "stale worktree", Quantity: 2},
				func(ctx context.Context) error {
					removed = 2 // e.g. `git worktree remove` for each stale path, honoring ctx
					return nil
				})
		})
		_ = out.finish()
		snap := out.snapshot()
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
	spec := EffectSpec{Verb: Delete, Object: "remote ref", Quantity: 3}
	fmt.Println(spec.Verb, spec.Quantity, spec.Object)
	// Output:
	// delete 3 remote ref
}

// ExampleEffectVerb lists the closed verb set. There is no write verb:
// file state goes through File, not an opaque Effect.
func ExampleEffectVerb() {
	for _, v := range []change.EffectVerb{change.EffectAdd, change.EffectCreate, change.EffectDelete, change.EffectPush, change.EffectRemove, change.EffectUpdate} {
		fmt.Print(v, " ")
	}
	fmt.Println()
	// Output:
	// add create delete push remove update
}

// ExampleEffectSpec_noResource shows the default: an EffectSpec with no
// Path or Resource makes no claim.
func ExampleEffectSpec_noResource() {
	spec := EffectSpec{Verb: Verb(change.EffectPush), Object: "branch", Quantity: 1}
	fmt.Println(spec.Path == "" && spec.Resource == "")
	// Output:
	// true
}

// ExamplePartialEffect reports a partial commit: 2 of 3 remote refs were
// deleted before the remote rejected the third. The ledger records the 2
// that really happened and the Task still fails with the cause.
func ExamplePartialEffect() {
	var buf strings.Builder
	out := initRun(Config{Name: "prune", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	rejected := errors.New("remote rejected refs/heads/old")
	var effectErr error
	out.Task("remote refs").Define(func(ctx context.Context) error {
		effectErr = Effect(ctx, EffectSpec{Verb: Delete, Object: "remote ref", Quantity: 3},
			func(ctx context.Context) error {
				committed := 2 // e.g. `git push --delete` for each ref until one fails
				return PartialEffect(committed, rejected)
			})
		return effectErr
	})
	_ = out.finish()
	snap := out.snapshot()
	fmt.Println(errors.Is(effectErr, rejected), snap.Changes[0].Records[0].Quantity)
	// Output:
	// true 2
}
