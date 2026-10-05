//go:build unix

package freshness_test

import (
	"path/filepath"
	"testing"
)

func TestC10_011_MissingBasisResourceIsHandledNotFatal(t *testing.T) {
	dir := t.TempDir()
	state, absent, out := filepath.Join(dir, "state"), filepath.Join(dir, "absent.txt"), filepath.Join(dir, "out.txt")
	gen := stage{name: "gen", script: "echo x > " + out, basis: []string{absent}, outputs: []string{out}}
	if !runStages(t, state, gen)["gen"] {
		t.Fatal("first run with a missing Basis input must execute")
	}
	if runStages(t, state, gen)["gen"] {
		t.Fatal("a still-missing Basis input is an unchanged identity, yet the work re-ran")
	}
	write(t, absent, "now present")
	if !runStages(t, state, gen)["gen"] {
		t.Fatal("a Basis input that appeared did not invalidate the result")
	}
}
