package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestInit_ZeroArgs_BuildsOrdinaryDefault is I9: evo.Init(evo.Config{}) (zero args)
// builds an ordinary default instance — construct.go's zero-config doc
// example (`out := evo.Init(evo.Config{})`) is real, not a Config{} literal in
// disguise. This test's mere compilation is most of the proof.
func TestInit_ZeroArgs_BuildsOrdinaryDefault(t *testing.T) {
	out := evo.Init(evo.Config{})
	if out == nil {
		t.Fatal("Init(Config{}) = nil")
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
