package evo

import (
	"bytes"
	"context"
	"testing"
)

// TestDefine_BlockThenNilRefuses pins the 1.1 Block-inside-Define form:
// Block then return nil concludes the run Blocked with exit 1.
func TestDefine_BlockThenNilRefuses(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "refuse", Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	task := out.Task("converge")
	task.Define(func(context.Context) error {
		task.block("needs review", Detail("ambiguous"))
		return nil
	})
	_ = out.finish()
	if c := out.conclusion(); c.headline() != headlineRefused || c.exitCode != exitRefused {
		t.Fatalf("conclusion = %s exit %d, want %s exit %d\n%s", c.headline(), c.exitCode, headlineRefused, exitRefused, buf.String())
	}
	_ = out.close()
}
