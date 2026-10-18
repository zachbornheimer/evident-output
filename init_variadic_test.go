package evo

import (
	"testing"
)

// TestInit_ZeroArgs_BuildsOrdinaryDefault is I9: initRun(Config{}) (zero args)
// builds an ordinary default instance — construct.go's zero-config doc
// example (`out := initRun(Config{})`) is real, not a Config{} literal in
// disguise. This test's mere compilation is most of the proof.
func TestInit_ZeroArgs_BuildsOrdinaryDefault(t *testing.T) {
	out := initRun(Config{})
	if out == nil {
		t.Fatal("initRun(Config{}) = nil")
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
}
