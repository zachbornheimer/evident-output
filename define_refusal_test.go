package evo_test

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDefine_BlockThenNilRefuses pins the 1.1 Block-inside-Define form:
// Block then return nil concludes the run Blocked with exit 1.
func TestDefine_BlockThenNilRefuses(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "refuse", Color: evo.ColorNever, Plain: true})
	task := out.Task("converge")
	task.Define(func(context.Context) error {
		task.Block("needs review", evo.Detail("ambiguous"))
		return nil
	})
	_ = out.Finish()
	if c := out.Conclusion(); c.State != evo.StateBlocked || c.ExitCode != evo.ExitBlocked {
		t.Fatalf("conclusion = %s exit %d, want %s exit %d\n%s", c.State, c.ExitCode, evo.StateBlocked, evo.ExitBlocked, buf.String())
	}
	_ = out.Close()
}
