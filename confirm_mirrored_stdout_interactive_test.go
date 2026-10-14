package evo

import (
	"bytes"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestFinish_InteractiveWithMultiWriterStdout_MirrorsPlainProjection proves the
// interactive branch of Finish honors alsoWrite: each extra writer receives
// the plain projection on Finish, with no carve-out for interactive runs.
func TestFinish_InteractiveWithMultiWriterStdout_MirrorsPlainProjection(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var mirror bytes.Buffer

	out := initRun(Config{stdout: io.Discard, stderr: io.Discard, terminal: screen, color: colorNever})
	out.alsoWriteForTest(&mirror)

	out.Task("dependencies").fail("dependency graph has a cycle")
	succeed(out.Task("build"))

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if mirror.Len() == 0 {
		t.Fatal("AlsoWrite mirror got nothing from an interactive Finish")
	}
}
