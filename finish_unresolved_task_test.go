package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_UnresolvedTaskWithRecordedEffect_AutoResolvesDone is
// beginner-finding-1's fix (a): a task that recorded a mutation-verb effect
// (Delete/Create/...) but was never given a terminal verb (Done/Fail/...)
// told an honest, complete story already — the easiest path (forgetting
// Done) becomes correct instead of a surprising Cancelled/NotStarted plus a
// silent exit-code flip to failure. This is exactly the README quickstart's
// `evo.Task("cleanup").Delete(2, "stale local branch")` shape with no
// following Done.
func TestFinish_UnresolvedTaskWithRecordedEffect_AutoResolvesDone(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("cleanup").Delete(2, "stale local branch")

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (recorded effect should auto-resolve Done)", err)
	}
	if code := out.Conclusion().ExitCode; code != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d (ExitOK)", code, evo.ExitOK)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, "✓") || !strings.Contains(rendered, "cleanup") {
		t.Fatalf("expected an auto-resolved Done row for cleanup, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "misuse") {
		t.Fatalf("no misuse expected once the task auto-resolves:\n%s", rendered)
	}
}

// TestFinish_RemainingMisuse_RendersTaskLine is beginner-finding-1's fix
// (b): any misuse that still changes the exit code (here, a mutation verb
// called on an already-Blocked task — the literal README quickstart bug)
// must render one line naming the task and a corrective hint, so a caller is
// never left staring at an exit code that contradicts everything the printed
// band showed. Release-gate round 4 finding 2 replaced the raw
// "misuse: <name>: evo: ..." sentinel dump this test used to pin with an
// honest, sentinel-specific hint — this pins the new line instead.
func TestFinish_RemainingMisuse_RendersTaskLine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("branches").Block("local-only branch")
	out.Task("branches").Delete(2, "stale local branch") // already resolved — misuse

	if err := out.Finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}
	rendered := buf.String()
	if strings.Contains(rendered, "evo: entity is already resolved") {
		t.Fatalf("raw sentinel jargon leaked into the user stream:\n%s", rendered)
	}
	wantHint := "resolve each task once; branches was already resolved"
	if !strings.Contains(rendered, "branches") || !strings.Contains(rendered, wantHint) {
		t.Fatalf("expected a rendered line naming the task and the corrective hint %q, got:\n%s", wantHint, rendered)
	}
}
