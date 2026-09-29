//go:build v06acceptance

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

	evo "github.com/zachbornheimer/evident-output"
)

// These assignments are the compile contract for §1.1 and §7/§9.1's public
// API. They intentionally stay separate from behavioral fixtures so
// implementation can promote the API in small steps.
var (
	_ = func() evo.RunFunc { return func(context.Context) error { return nil } }
	_ = func() func(context.Context, evo.RunFunc) evo.Result {
		var out *evo.Output
		return out.Run
	}
	_ = func() func(evo.RunFunc) int { return evo.Main }
	_ = func() func(context.Context, evo.RunFunc) evo.Result { return evo.Run }
)

func TestV06PublicTypesCompile(t *testing.T) {
	var _ evo.Config = evo.Config{
		Title:     "contract",
		Format:    evo.FormatHuman,
		Verbosity: evo.VerbosityNormal,
		DryRun:    true,
		Stdout:    nil,
		Stderr:    nil,
		Isolated:  true,
	}
}

func TestV06TaskContextSurfaceCompiles(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("context")
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	task.Define(func(context.Context) error { return nil })
}
