package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestBlockedInsideContainer_ConcludesBlocked pins E-098: a Task Blocked
// inside a Group or Sequence concluded "[ready]" with exit 0, because the
// Conclusion read a container's Blocked state as nothing. A refusal is a
// refusal at any depth.
func TestBlockedInsideContainer_ConcludesBlocked(t *testing.T) {
	for name, declare := range map[string]func(out *evo.Output){
		"group":         func(out *evo.Output) { out.Group("g").Task("a").Block("refused") },
		"sequence":      func(out *evo.Output) { out.Sequence("s").Task("a").Block("refused") },
		"group summary": func(out *evo.Output) { out.Group("g").Summary("x").Task("a").Block("refused") },
		"nested":        func(out *evo.Output) { out.Group("g").Sequence("s").Task("a").Block("refused") },
	} {
		var buf bytes.Buffer
		out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "block", Color: evo.ColorNever, Plain: true})
		declare(out)
		_ = out.Finish()
		c := out.Conclusion()
		if c.State != evo.StateBlocked || c.ExitCode != evo.ExitBlocked {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.State, c.ExitCode, evo.StateBlocked, evo.ExitBlocked, buf.String())
		}
		if !strings.Contains(buf.String(), "[blocked]") {
			t.Errorf("%s: output lacks the [blocked] band:\n%s", name, buf.String())
		}
		_ = out.Close()
	}
}
