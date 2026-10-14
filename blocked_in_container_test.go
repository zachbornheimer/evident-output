package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestBlockedInsideContainer_ConcludesBlocked pins E-098: a Task Blocked
// inside a Group or Sequence concluded "[succeeded]" with exit 0, because the
// Conclusion read a container's Blocked state as nothing. A refusal is a
// refusal at any depth.
func TestBlockedInsideContainer_ConcludesBlocked(t *testing.T) {
	for name, declare := range map[string]func(out *Run){
		"group":         func(out *Run) { out.Group("g").Task("a").block("refused") },
		"sequence":      func(out *Run) { out.Sequence("s").Task("a").block("refused") },
		"group summary": func(out *Run) { out.Group("g").Summary("x").Task("a").block("refused") },
		"nested":        func(out *Run) { out.Group("g").Sequence("s").Task("a").block("refused") },
	} {
		var buf bytes.Buffer
		out := initRun(Config{stateDir: t.TempDir(), stdout: &buf, title: "block", color: colorNever, plain: true})
		declare(out)
		_ = out.finish()
		c := out.conclusion()
		if c.headline() != headlineRefused || c.exitCode != exitRefused {
			t.Errorf("%s: conclusion = %s exit %d, want %s exit %d\n%s", name, c.headline(), c.exitCode, headlineRefused, exitRefused, buf.String())
		}
		if !strings.Contains(buf.String(), "[refused]") {
			t.Errorf("%s: output lacks the [refused] band:\n%s", name, buf.String())
		}
		_ = out.close()
	}
}
