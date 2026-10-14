package evo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A dry run runs every Define callback and plans every mutation: the Define
// that records a side marker runs, the Effect callback inside it does not, and
// a File is planned, never written. This is the behaviour CONTRACT.md's dry-run
// sentence states.
func TestDryRunRunsDefineCallbacksAndSkipsEffects(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	target := filepath.Join(t.TempDir(), "planned.txt")
	defineRan, effectRan := false, false

	out := initRun(Config{Name: "retire", DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	out.Task("work").Define(func(ctx context.Context) error {
		defineRan = true
		if err := File(ctx, FileSpec{Path: target, Contents: []byte("x"), Mode: 0o644}); err != nil {
			return err
		}
		return Effect(ctx, EffectSpec{Verb: Delete, Object: "stale local branch", Quantity: 2}, func(context.Context) error {
			effectRan = true
			return nil
		})
	})
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if !defineRan {
		t.Fatal("dry run skipped the Define callback; it must run it")
	}
	if effectRan {
		t.Fatal("dry run ran the Effect callback; it must only plan it")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote the File (stat err = %v); it must only plan it", err)
	}
	got := strings.Join(strings.Fields(buf.String()), " ")
	if !strings.Contains(got, "[planned] work write "+target) || strings.Contains(got, "[changed]") {
		t.Fatalf("want the File planned and no [changed], got:\n%s", buf.String())
	}
	if !strings.Contains(got, "delete 2 stale local branches") {
		t.Fatalf("want the planned Effect row, got:\n%s", buf.String())
	}
}
