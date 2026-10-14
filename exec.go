package evo

import (
	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/freshness"
)

type ExecSpec struct {
	Argv       []string
	Dir        string
	Env        map[string]string
	Inputs     []string
	Values     map[string]string
	Outputs    []string
	executable string
	args       []string
	basis      []freshness.Fingerprint
}

func (s ExecSpec) toChange() change.ExecSpec {
	cs := change.ExecSpec{Dir: s.Dir, Env: s.Env, Outputs: s.Outputs, Basis: freshness.SpecBasis(s.Inputs, s.Values, s.basis)}
	if len(s.Argv) > 0 {
		cs.Executable = s.Argv[0]
		cs.Args = s.Argv[1:]
	} else {
		cs.Executable = s.executable
		cs.Args = s.args
	}
	return cs
}

type ExecResult struct {
	Ran       bool
	ExitCode  int
	Stdout    string
	Stderr    string
	Truncated bool
}
