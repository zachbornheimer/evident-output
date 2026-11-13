package evo_test

import (
	"bytes"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestFinish_InteractiveWithAlsoWrite_MirrorsPlainProjection proves the
// interactive branch of Finish honors alsoWrite: each extra writer receives
// the plain projection on Finish, with no carve-out for interactive runs.
func TestFinish_InteractiveWithAlsoWrite_MirrorsPlainProjection(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var mirror bytes.Buffer

	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, Color: evo.ColorNever})
	out.AlsoWriteForTest(&mirror)

	out.Task("dependencies").Fail("dependency graph has a cycle")
	out.Task("build").Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if mirror.Len() == 0 {
		t.Fatal("AlsoWrite mirror got nothing from an interactive Finish")
	}
}
