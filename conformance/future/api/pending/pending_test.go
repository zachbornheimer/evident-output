//go:build v06acceptance

// Package pending_test holds the v0.6/1.0 public API compile contract for
// increment 2 and later: File/Exec/FileSpec/ExecSpec and
// Config.AppID/Config.StateDir. None of it exists in this worktree yet
// (increment 1's blast radius explicitly excludes File/Exec/fingerprints/
// manifest work) — this package is deliberately red-by-compile under
// `go test -tags v06acceptance ./conformance/future/api/pending` until a
// later increment implements the missing symbols. It is a separate Go
// package from ../api_contract_test.go specifically so this package's
// compile failure never blocks that one's tests from running.
package pending_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

var _ = func() func(context.Context, evo.FileSpec) error { return evo.File }

func TestV06PendingPublicTypesCompile(t *testing.T) {
	var _ evo.Config = evo.Config{
		Title:    "contract",
		AppID:    "example",
		StateDir: t.TempDir(),
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Isolated: true,
	}
	var _ evo.FileSpec = evo.FileSpec{Path: "out", Contents: []byte{}, Mode: fs.FileMode(0o644), Basis: []evo.Fingerprint{evo.FSPath("in")}}
	var _ evo.ExecSpec = evo.ExecSpec{Executable: "tool", Args: []string{"arg"}, Env: map[string]string{"KEY": "value"}, Outputs: []string{"out"}}
	var _ = errors.Is
}
