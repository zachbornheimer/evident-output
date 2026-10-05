package facts_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func effect(verb evo.EffectVerb, object string, quantity int) func(context.Context) error {
	return func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: verb, Object: object, Quantity: quantity},
			func(context.Context) error { return nil })
	}
}

func TestC12_001_ContentFreeEffectIsRefusedAndRecordsNothing(t *testing.T) {
	out, buf := harness.New(t)
	noop := func(context.Context) error { return nil }
	cases := map[string]struct {
		spec evo.EffectSpec
		fn   func(context.Context) error
		want error
	}{
		"object":   {evo.EffectSpec{Verb: evo.EffectDelete, Quantity: 1}, noop, evo.ErrEffectObjectMissing},
		"quantity": {evo.EffectSpec{Verb: evo.EffectDelete, Object: "tip"}, noop, evo.ErrEffectQuantityNotPositive},
		"verb":     {evo.EffectSpec{Verb: "frobnicate", Object: "tip", Quantity: 1}, noop, evo.ErrEffectVerbInvalid},
		"callback": {evo.EffectSpec{Verb: evo.EffectDelete, Object: "tip", Quantity: 1}, nil, evo.ErrEffectCallbackMissing},
	}
	task := out.Task("job")
	got := map[string]error{}
	task.Define(func(ctx context.Context) error {
		for name, c := range cases {
			got[name] = evo.Effect(ctx, c.spec, c.fn)
		}
		return nil
	})
	_ = task.Wait()
	for name, c := range cases {
		if !errors.Is(got[name], c.want) {
			t.Errorf("%s: err = %v, want %v", name, got[name], c.want)
		}
	}
	if text := harness.Text(out, buf); strings.Contains(text, "[changed]") || strings.Contains(text, "[planned]") {
		t.Fatalf("a refused Effect left a ledger row:\n%s", text)
	}
}

func TestC12_002_AggregateRowsUseSubjectVerbQuantityObject(t *testing.T) {
	changed, buf := harness.New(t)
	_ = changed.Task("prune").Define(effect(evo.EffectDelete, "branch", 3)).Wait()
	if text := harness.Text(changed, buf); !strings.Contains(text, "[changed] prune  deleted 3 branches") {
		t.Fatalf("apply:\n%s", text)
	}
	planned, pbuf := harness.New(t, func(c *evo.Config) { c.DryRun = true })
	_ = planned.Task("prune").Define(effect(evo.EffectDelete, "branch", 3)).Wait()
	if text := harness.Text(planned, pbuf); !strings.Contains(text, "[planned] prune  delete 3 branches") {
		t.Fatalf("dry-run:\n%s", text)
	}
}

func TestC12_003_RowsPrintInDeclarationOrderNotFinishOrder(t *testing.T) {
	out, buf := harness.New(t)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i, name := range []string{"first", "second", "third"} {
		task := out.Task(name)
		wg.Add(1)
		task.Define(func(ctx context.Context) error {
			defer wg.Done()
			if i < 2 {
				<-release
			}
			return effect(evo.EffectDelete, "tip", 1)(ctx)
		})
	}
	close(release)
	wg.Wait()
	text := harness.Text(out, buf)
	a, b, c := strings.Index(text, "[changed] first"), strings.Index(text, "[changed] second"), strings.Index(text, "[changed] third")
	if a < 0 || b < a || c < b {
		t.Fatalf("ledger order:\n%s", text)
	}
}

func TestC12_004_AmbiguousSectionNamesAreQualifiedByContainerPath(t *testing.T) {
	out, buf := harness.New(t)
	for _, group := range []string{"alpha", "beta"} {
		g := out.Group(group)
		g.Task("prune").Define(effect(evo.EffectDelete, "branch", 2))
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	text := harness.Text(out, buf)
	if !strings.Contains(text, "alpha › prune") || !strings.Contains(text, "beta › prune") {
		t.Fatalf("sections not qualified:\n%s", text)
	}
}

func TestC12_005_SubjectColumnsAlignWithExactlyTwoSpaces(t *testing.T) {
	out, buf := harness.New(t)
	_ = out.Task("a").Define(effect(evo.EffectDelete, "tip", 1)).Wait()
	_ = out.Task("remote-tracking").Define(effect(evo.EffectDelete, "ref", 4)).Wait()
	text := harness.Text(out, buf)
	short := strings.Index(text, "deleted 1 tip")
	long := strings.Index(text, "deleted 4 refs")
	if short < 0 || long < 0 || !strings.Contains(text, "[changed] remote-tracking  deleted 4 refs") {
		t.Fatalf("ledger:\n%s", text)
	}
	lineStart := func(i int) int { return strings.LastIndex(text[:i], "\n") + 1 }
	if short-lineStart(short) != long-lineStart(long) {
		t.Fatalf("verb columns differ:\n%s", text)
	}
}
