//go:build ignore

// Package futureapi_test holds increment-1's public API compile contract —
// symbols already implemented in this worktree. It compiles and passes
// under `go test -tags v06acceptance ./conformance/future/api`; increment
// 2+ surface (File/Exec/FileSpec/ExecSpec/Config.AppID/Config.StateDir)
// lives in the sibling ./pending package instead, which stays red-by-
// compile until those increments land — keeping this package's own compile
// (and therefore its tests) independent of that missing surface.
package futureapi_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// These assignments are the compile contract for §1.1 and §7/§9.1's public
// API. They intentionally stay separate from behavioral fixtures so
// implementation can promote the API in small steps.
var (
	_ = func() evo.RunFunc { return func(context.Context) error { return nil } }
	_ = func() func(context.Context, evo.RunFunc) evo.Result {
		var out *evo.Run
		return out.Run
	}
	_ = func() func(evo.RunFunc) int { return evo.Main }
	_ = func() func(context.Context, evo.RunFunc) evo.Result { return evo.Run }
)

func TestV06PublicTypesCompile(t *testing.T) {
	var _ evo.Config = evo.Config{Name: "contract", DryRun: true, Output: evo.OutputHuman, Verbose: evo.VerbosityNormal, Facades: evo.Facades{Stdout: nil, Stderr: nil}}
}

func TestV06TaskContextSurfaceCompiles(t *testing.T) {
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	task := out.Task("context")
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	task.Define(func(context.Context) error { return nil })
}
