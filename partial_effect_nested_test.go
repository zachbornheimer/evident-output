package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPartialEffect_BindsToTheEffectWhoseCallbackReturnedIt proves an
// outer Effect never counts a PartialEffect an inner Effect already
// recorded: the inner delete committed 2 of 3 refs, the outer push
// committed nothing, and the ledger must say exactly that.
func TestPartialEffect_BindsToTheEffectWhoseCallbackReturnedIt(t *testing.T) {
	t.Parallel()
	cause := errors.New("ref 3 locked")
	push := evo.EffectSpec{Verb: evo.EffectPush, Object: "branch", Quantity: 5}
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "sync", Color: evo.ColorNever, Plain: true, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	var effectErr error
	task := out.Task("sync")
	task.Define(func(ctx context.Context) error {
		effectErr = evo.Effect(ctx, push, func(ctx context.Context) error {
			return evo.Effect(ctx, refDelete, func(context.Context) error {
				return evo.PartialEffect(2, cause)
			})
		})
		return effectErr
	})
	_ = out.Finish()
	if !errors.Is(effectErr, cause) {
		t.Fatalf("Effect err = %v, want the cause kept reachable", effectErr)
	}
	var objects []string
	for _, c := range out.Snapshot().Changes {
		for _, r := range c.Records {
			objects = append(objects, r.Object)
		}
	}
	if got := changedQuantities(out.Snapshot()); len(got) != 1 || got[0] != 2 || objects[0] != "remote ref" {
		t.Fatalf("changed records = %v %v, want only the inner 2 remote refs:\n%s", objects, got, buf.String())
	}
	if strings.Contains(buf.String(), "pushed") {
		t.Fatalf("ledger invents pushed branches the outer Effect never committed:\n%s", buf.String())
	}
}

// TestPartialEffect_JoinedAfterAnInnerOneStillCounts proves an outer
// Effect's own PartialEffect is recorded even when its callback joins it
// after an inner Effect's already-recorded one.
func TestPartialEffect_JoinedAfterAnInnerOneStillCounts(t *testing.T) {
	t.Parallel()
	push := evo.EffectSpec{Verb: evo.EffectPush, Object: "branch", Quantity: 5}
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "sync", Color: evo.ColorNever, Plain: true, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("sync")
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, push, func(ctx context.Context) error {
			inner := evo.Effect(ctx, refDelete, func(context.Context) error {
				return evo.PartialEffect(1, errors.New("ref 2 locked"))
			})
			return errors.Join(inner, evo.PartialEffect(4, errors.New("branch 5 rejected")))
		})
	})
	_ = out.Finish()
	var got []string
	for _, c := range out.Snapshot().Changes {
		for _, r := range c.Records {
			got = append(got, fmt.Sprintf("%s %d %s", r.Verb, r.Quantity, r.Object))
		}
	}
	slices.Sort(got)
	want := []string{"deleted 1 remote ref", "pushed 4 branch"}
	if !slices.Equal(got, want) {
		t.Fatalf("changed records = %q, want %q:\n%s", got, want, buf.String())
	}
}
