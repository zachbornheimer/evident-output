package evo_test

import (
	"bytes"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestFinish_InteractiveWithAlsoWrite_MirrorsPlainProjection proves the
// interactive branch of Finish honors AlsoWrite (X4): option.go promises
// "each [AlsoWrite] writer receives the plain projection" on Finish, with no
// carve-out for interactive runs. Before the fix, Finish's interactive path
// only ever considered cfg.primary for a dual-stream write — an AlsoWrite
// mirror (e.g. a log file alongside a live terminal) got nothing.
func TestFinish_InteractiveWithAlsoWrite_MirrorsPlainProjection(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var mirror bytes.Buffer

	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.Terminal(screen),
		evo.AlsoWrite(&mirror),
	}})

	// Two tasks (not one) keep the conclusion band from coalescing into the
	// single task's own row — residualPlainLocked's writeConclusion call is
	// guaranteed non-empty, isolating this test to the AlsoWrite fan-out bug.
	out.Task("dependencies").Fail("dependency graph has a cycle")
	out.Task("build").Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if mirror.Len() == 0 {
		t.Fatal("AlsoWrite mirror got nothing from an interactive Finish")
	}
}
