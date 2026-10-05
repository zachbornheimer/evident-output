package evo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestProblem_DefaultSeverityFailsTaskAndRun(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("check")
	task.Define(func(context.Context) error {
		task.Problem("x")
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("task state = %s, want Failed", got)
	}
	c := out.Conclusion()
	if c.State != evo.StateFailed || c.ExitCode != evo.ExitFailed {
		t.Fatalf("conclusion = %s exit %d, want failed exit %d\n%s", c.State, c.ExitCode, evo.ExitFailed, buf.String())
	}
}

func TestProblem_WarningSeveritySettlesDoneAndWarns(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("check")
	task.Define(func(context.Context) error {
		task.Problem("x", evo.Severity(evo.SeverityWarning))
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := task.Snapshot()
	if snap.State != evo.Done {
		t.Fatalf("task state = %s, want Done", snap.State)
	}
	if len(snap.Warnings) != 1 || snap.Warnings[0].Summary != "x" {
		t.Fatalf("warnings = %#v, want one warning x", snap.Warnings)
	}
	if len(snap.Problems) != 0 {
		t.Fatalf("problems = %#v, want none on a warning Problem", snap.Problems)
	}
	c := out.Conclusion()
	if !c.Warned || c.ExitCode != evo.ExitOK {
		t.Fatalf("conclusion warned=%v exit %d, want warned exit 0\n%s", c.Warned, c.ExitCode, buf.String())
	}
}

func TestProblem_InvalidSeverityIsRejected(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("check")
	task.Define(func(context.Context) error {
		task.Problem("x", evo.Severity("nope"))
		return nil
	})
	_ = out.Finish()
	snap := task.Snapshot()
	if len(snap.Problems) != 0 || len(snap.Warnings) != 0 {
		t.Fatalf("invalid severity recorded a problem: problems=%#v warnings=%#v", snap.Problems, snap.Warnings)
	}
	err := out.Err()
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Err() = %v, want a context-bearing invalid severity error containing nope", err)
	}
}
