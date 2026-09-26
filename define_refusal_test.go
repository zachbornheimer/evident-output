package evo_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDefine_BlockInsideDefineRefuses pins the runtime half of E-105: a
// Define callback that calls Block(...) and returns nil concludes the run
// Blocked with exit 1 — the same verdict the original Block statement gave.
func TestDefine_BlockInsideDefineRefuses(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "refuse", Color: evo.ColorNever, Plain: true})
	task := out.Task("converge")
	task.Define(func(context.Context) error {
		task.Block("needs review: " + errors.New("ambiguous").Error())
		return nil
	})
	_ = out.Finish()
	if c := out.Conclusion(); c.State != evo.StateBlocked || c.ExitCode != evo.ExitBlocked {
		t.Fatalf("conclusion = %s exit %d, want %s exit %d\n%s", c.State, c.ExitCode, evo.StateBlocked, evo.ExitBlocked, buf.String())
	}
	_ = out.Close()
}
