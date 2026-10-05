//go:build evopending

// Pending: ZYS-1370 ledger fold. No new API: the renderer folds per-item
// Effects that share one owner identity into one aggregated human row,
// while Snapshot/JSON/JSONL keep every Effect and failed items stay visible.
// Compiles today; red because the human projection prints one row per item.
package ledger_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func deleteBranch(out *evo.Output, group *evo.GroupHandle, name string, fail bool) {
	group.Task("delete " + name).Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "branch", Quantity: 1}, func(context.Context) error {
			if fail {
				return errors.New("ref is protected")
			}
			return nil
		})
	})
}

func runFoldFixture(t *testing.T, failLast bool) (*evo.Output, string) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true, Color: evo.ColorNever, Title: "prune"})
	t.Cleanup(func() { _ = out.Close() })
	group := out.Group("branches")
	deleteBranch(out, group, "a", false)
	deleteBranch(out, group, "b", false)
	deleteBranch(out, group, "c", failLast)
	_ = out.Finish()
	return out, buf.String()
}

func TestPerItemEffectsUnderOneGroupFoldIntoOneHumanRow(t *testing.T) {
	_, human := runFoldFixture(t, false)
	if got := strings.Count(human, "deleted 1 branch"); got != 0 {
		t.Fatalf("human output prints %d per-item rows, want them folded:\n%s", got, human)
	}
	if !strings.Contains(human, "deleted 3 branches") {
		t.Fatalf("human output lacks the aggregated row:\n%s", human)
	}
}

func TestFoldKeepsEveryEffectInTheSnapshot(t *testing.T) {
	out, _ := runFoldFixture(t, false)
	var records int
	for _, section := range out.Snapshot().Changes {
		records += len(section.Records)
	}
	if records != 3 {
		t.Fatalf("snapshot keeps %d Effect records, want 3", records)
	}
}

func TestFoldKeepsFailedItemsVisible(t *testing.T) {
	_, human := runFoldFixture(t, true)
	if !strings.Contains(human, "delete c") || !strings.Contains(human, "ref is protected") {
		t.Fatalf("failed item disappeared into the fold:\n%s", human)
	}
}
