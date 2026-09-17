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

var _ = func() func(context.Context, evo.ExecSpec) error { return evo.Exec }

type noopRunner struct{}

func (noopRunner) Run(context.Context, evo.ProcessCommand) (evo.ProcessOutcome, error) {
	return evo.ProcessOutcome{}, nil
}

func TestV06ExecAPITypesCompile(t *testing.T) {
	var _ evo.ExecSpec = evo.ExecSpec{
		Executable: "tool",
		Args:       []string{"arg"},
		Dir:        "workdir",
		Env:        map[string]string{"KEY": "value"},
		Basis:      []evo.Fingerprint{evo.FSPath("in")},
		Outputs:    []string{"out"},
	}
	var _ evo.Config = evo.Config{ProcessRunner: noopRunner{}}
	var _ evo.Option = evo.Runner(noopRunner{})
}
