package evo_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTaskRun_PlainMode_NoPerLineDurableRows is release-gate round 9 finding
// 4's red case: off a TTY, a talkative child's PhaseWriter/Task.Run-mirrored
// output lines must not each force their own durable "running" row — the
// child's full output already has one durable home (the Evidence ring and
// its failure-path DetailTail). Before the fix, every mirrored line reached
// the SAME TaskHandle.Phase path an explicit caller call uses, so plain
// mode's per-line-phase-change streaming (the P10 contract) fired once per
// child line too.
func TestTaskRun_PlainMode_NoPerLineDurableRows(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		Options: []evo.Option{
			evo.To(&buf),
			evo.Plain(),
			evo.NoColor(),
		},
	})

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "printf 'compiling a.go\\ncompiling b.go\\ncompiling c.go\\ncompiling d.go\\n'")
	if err := task.Run(cmd); err != nil {
		t.Fatalf("task.Run: %v", err)
	}
	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if n := strings.Count(got, "compiling"); n != 0 {
		t.Fatalf("want zero durable rows for the child's mirrored per-line output, got %d occurrences:\n%s", n, got)
	}
	if !strings.Contains(got, "✓") {
		t.Fatalf("want the task's resolved ✓ row, got:\n%s", got)
	}
}

// TestTaskRun_PlainMode_FailureShowsTailOnce is the failure-path sibling:
// the child's output must still surface exactly once, through the failure
// DetailTail — never duplicated by an extra per-line durable stream.
func TestTaskRun_PlainMode_FailureShowsTailOnce(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		Options: []evo.Option{
			evo.To(&buf),
			evo.Plain(),
			evo.NoColor(),
		},
	})

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo 'undefined reference to main' 1>&2; exit 1")
	if err := task.Run(cmd); err != nil {
		_ = task.Failf("build failed: %w", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a resolved Fail is not misuse)", err)
	}

	got := buf.String()
	if n := strings.Count(got, "undefined reference to main"); n != 1 {
		t.Fatalf("want the child's stderr exactly once, got %d occurrences:\n%s", n, got)
	}
}
