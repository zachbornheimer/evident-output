//go:build v06acceptance

// Package pending_test holds the v0.6/1.0 public API compile contract for
// increment 3 (Exec): ExecSpec/evo.Exec. It is deliberately red-by-compile
// under `go test -tags v06acceptance ./conformance/future/api/pending`
// until that increment implements the missing symbols. File's own contract
// moved to ../file (increment 2, green) — see that package for
// File/FileSpec/Config.AppID/Config.StateDir.
package pending_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestV06PendingPublicTypesCompile(t *testing.T) {
	var _ evo.ExecSpec = evo.ExecSpec{
		Executable: "tool",
		Args:       []string{"arg"},
		Env:        map[string]string{"KEY": "value"},
		Outputs:    []string{"out"},
	}
}
