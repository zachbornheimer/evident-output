package evo_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// claimHold is how long each Effect callback holds its claim, long enough
// that two unclaimed callbacks in one Group would visibly overlap.
const claimHold = 150 * time.Millisecond

// TestEffectPathClaimsFilesystem pins CONTRACT.md §3: EffectSpec.Path claims
// that filesystem path, so Effects whose Paths are equal or nested never run
// at once, and a spec that sets both Path and Resource is ErrInvalidConfig.
func TestEffectPathClaimsFilesystem(t *testing.T) {
	t.Run("nested paths never overlap", func(t *testing.T) {
		dir := t.TempDir()
		paths := []string{dir, filepath.Join(dir, "sub", "file.txt"), dir}
		var inside, most atomic.Int32
		run := executeContract(context.Background(), t,
			evo.Config{Name: "claims", Output: evo.Plain, MaxConcurrency: len(paths)},
			func(_ context.Context, r *evo.Run) error {
				g := r.Group("writers")
				for i, path := range paths {
					g.Task(string(rune('a' + i))).Define(func(ctx context.Context) error {
						spec := evo.EffectSpec{Verb: evo.Update, Object: "file", Quantity: 1, Path: path}
						return evo.Effect(ctx, spec, func(context.Context) error {
							now := inside.Add(1)
							for prev := most.Load(); now > prev && !most.CompareAndSwap(prev, now); prev = most.Load() {
							}
							time.Sleep(claimHold)
							inside.Add(-1)
							return nil
						})
					})
				}
				return nil
			})
		if code := run.Result.ExitCode(); code != 0 {
			t.Fatalf("run exited %d:\n%s", code, run.Text())
		}
		if got := most.Load(); got != 1 {
			t.Errorf("%d Effect callbacks with nested Paths ran at once, want 1", got)
		}
	})

	t.Run("Path with Resource is ErrInvalidConfig", func(t *testing.T) {
		spec := evo.EffectSpec{Verb: evo.Update, Object: "row", Quantity: 1, Path: t.TempDir()}
		setResource(t, &spec, "db")
		var ran atomic.Bool
		var effectErr error
		run := executeContract(context.Background(), t, evo.Config{Name: "claims", Output: evo.Plain},
			func(_ context.Context, r *evo.Run) error {
				r.Task("a").Define(func(ctx context.Context) error {
					effectErr = evo.Effect(ctx, spec, func(context.Context) error {
						ran.Store(true)
						return nil
					})
					return effectErr
				})
				return nil
			})
		if ran.Load() {
			t.Error("the callback of an Effect with both Path and Resource ran")
		}
		if effectErr == nil || !strings.Contains(effectErr.Error(), "invalid configuration") {
			t.Errorf("Effect returned %v, want ErrInvalidConfig", effectErr)
		}
		if run.Result.ExitCode() == 0 {
			t.Errorf("run exited 0; an invalid EffectSpec must not succeed:\n%s", run.Text())
		}
	})
}

// setResource sets spec.Resource to the logical claim name. CONTRACT.md §3
// gives EffectSpec.Resource as a logical claim a consumer sets; this sets it
// through reflection so the test compiles whatever the field's type is today,
// and fails when a consumer cannot set it from a name.
func setResource(t *testing.T, spec *evo.EffectSpec, name string) {
	t.Helper()
	field := reflect.ValueOf(spec).Elem().FieldByName("Resource")
	if field.Kind() != reflect.String {
		t.Fatalf("EffectSpec.Resource has type %s; a consumer cannot set a logical claim from a name", field.Type())
	}
	field.Set(reflect.ValueOf(name).Convert(field.Type()))
}
