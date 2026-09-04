package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_ForgottenTerminalVerb_SamePolicyWithOrWithoutPhase is the
// red-first case for release-gate round 4 finding 3: whether the caller
// called Phase before abandoning the task must not flip the verdict class or
// the exit code — both programs are a clean, unsignalled finish with a
// forgotten terminal verb and zero problems, asserted on rendered bytes.
func TestFinish_ForgottenTerminalVerb_SamePolicyWithOrWithoutPhase(t *testing.T) {
	run := func(callPhase bool) (rendered string, exitCode int) {
		var buf bytes.Buffer
		out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
		task := out.Task("install")
		if callPhase {
			task.Phase("working")
		}
		_ = out.Finish()
		return buf.String(), out.Conclusion().ExitCode
	}

	withPhase, withPhaseExit := run(true)
	withoutPhase, withoutPhaseExit := run(false)

	if withPhaseExit != withoutPhaseExit {
		t.Fatalf("exit code depends on whether Phase was called: with=%d without=%d", withPhaseExit, withoutPhaseExit)
	}
	if withPhaseExit != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d (ExitOK) for a clean finish with a forgotten terminal verb", withPhaseExit, evo.ExitOK)
	}
	if !strings.Contains(withPhase, "· partial]") || !strings.Contains(withoutPhase, "· partial]") {
		t.Fatalf("both renders must carry the partial modifier; with-phase:\n%s\nwithout-phase:\n%s", withPhase, withoutPhase)
	}
}
