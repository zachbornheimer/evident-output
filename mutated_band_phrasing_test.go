package evo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestMutatedBandUsesLedgerPhrasing pins that the "already mutated" band says each effect the way the
// ledger does, verb first: "deleted 1 branch" and "wrote in.txt", never "1 branch deleted", and never
// "write in: wrote in.txt", where the task name only repeats what the effect already names.
func TestMutatedBandUsesLedgerPhrasing(t *testing.T) {
	inFile := filepath.Join(t.TempDir(), "in.txt")
	run := executeContract(context.Background(), t,
		evo.Config{Name: "band", Output: evo.Plain, State: evo.State{Dir: t.TempDir(), AppID: "contract"}},
		func(_ context.Context, r *evo.Run) error {
			r.Task("write in").Define(func(ctx context.Context) error {
				return evo.File(ctx, evo.FileSpec{Path: inFile, Contents: []byte("v=1\n"), Mode: 0o644})
			})
			r.Task("branches").Define(func(ctx context.Context) error {
				spec := evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 1}
				return evo.Effect(ctx, spec, func(context.Context) error { return nil })
			})
			r.Task("lint").Define(func(context.Context) error { return evo.Fail("lint failed") })
			return nil
		})

	band := ""
	for line := range strings.SplitSeq(run.Stderr, "\n") {
		if strings.Contains(line, "already mutated:") {
			band = line
		}
	}
	if band == "" {
		t.Fatalf("no already-mutated band:\n%s", run.Stderr)
	}
	for _, want := range []string{"wrote in.txt", "deleted 1 branch"} {
		if !strings.Contains(band, want) {
			t.Errorf("band %q does not say %q", band, want)
		}
	}
	for _, unwanted := range []string{"write in:", "1 branch deleted"} {
		if strings.Contains(band, unwanted) {
			t.Errorf("band %q says %q, which the ledger never does", band, unwanted)
		}
	}
}
