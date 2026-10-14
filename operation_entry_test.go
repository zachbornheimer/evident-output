package evo

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/change"
)

// TestEveryOperationRefusesACancelledContextTheSameWay proves the five
// Define-scoped operations share one entry policy: each refuses a done ctx
// with an error wrapping context.Canceled and records it where Output.Err
// reports it. Effect and Patch used to return the error and record
// nothing, and Files checked nothing, so Output.Err depended on the verb.
func TestEveryOperationRefusesACancelledContextTheSameWay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	ops := map[string]func(ctx context.Context) error{
		"Effect": func(ctx context.Context) error {
			return Effect(ctx, EffectSpec{Verb: Delete, Object: "branch", Quantity: 1}, func(context.Context) error { return nil })
		},
		"File":  func(ctx context.Context) error { return File(ctx, FileSpec{Path: path, Contents: []byte("x")}) },
		"Files": func(ctx context.Context) error { return files(ctx, change.FileSet{}) },
		"Exec": func(ctx context.Context) error {
			_, err := Exec(ctx, ExecSpec{executable: "true"})
			return err
		},
		"Patch": func(ctx context.Context) error {
			err := Patch(ctx, nil)
			return err
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			out := initRun(Config{stateDir: t.TempDir(), stdout: io.Discard, stderr: io.Discard})
			t.Cleanup(func() { _ = out.close() })
			var opErr error
			task := out.Task("op")
			task.Define(func(ctx context.Context) error {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				opErr = op(cancelled)
				return nil
			})
			_ = task.wait()
			if !errors.Is(opErr, context.Canceled) {
				t.Fatalf("%s on a cancelled ctx = %v, want context.Canceled", name, opErr)
			}
			if !errors.Is(out.err(), context.Canceled) {
				t.Fatalf("%s: Output.Err() = %v, want the recorded cancellation", name, out.err())
			}
		})
	}
}
