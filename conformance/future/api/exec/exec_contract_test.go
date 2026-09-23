//go:build v06acceptance

// Package exec_test holds increment 3's v0.6/1.0 public API compile
// contract: ExecSpec/evo.Exec, evo.ProcessRunner/ProcessCommand/
// ProcessOutcome, and the Runner Option (spec §8.4). Split out of
// ../pending/pending_test.go (now removed — increment 3 is the last
// increment/pending tranche this future suite carries) now that evo.Exec
// exists.
package exec_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

var _ = func() func(context.Context, evo.ExecSpec) (evo.ExecResult, error) { return evo.Exec }

var _ = func() func(evo.ProcessRunner) evo.Option { return evo.Runner }

type noopRunner struct{}

func (noopRunner) Run(context.Context, evo.ProcessCommand) (evo.ProcessOutcome, error) {
	return evo.ProcessOutcome{}, nil
}

func TestV06ExecAPITypesCompile(t *testing.T) {
	_ = evo.ExecSpec{
		Executable: "tool",
		Args:       []string{"arg"},
		Dir:        "workdir",
		Env:        map[string]string{"KEY": "value"},
		Basis:      []evo.Fingerprint{evo.FSPath("in")},
		Outputs:    []string{"out"},
	}
	_ = evo.Config{ProcessRunner: noopRunner{}}
	_ = evo.Runner(noopRunner{})
}
