package evo_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The §39 propagation-stop metric for Exec: a Basis change forces the
// command to rerun, but it regenerates a byte-identical output, so the
// operation finishes unchanged. A dry run never runs the command, so it
// cannot claim the output changed.

type execWorkspace struct {
	state, source, output string
}

func newExecWorkspace(t *testing.T) execWorkspace {
	t.Helper()
	dir := t.TempDir()
	ws := execWorkspace{state: t.TempDir(), source: filepath.Join(dir, "gen.src"), output: filepath.Join(dir, "gen.out")}
	ws.editSource(t, "v1")
	return ws
}

func (ws execWorkspace) editSource(t *testing.T, text string) {
	t.Helper()
	if err := os.WriteFile(ws.source, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// run executes one generator Exec whose output ignores its Basis content.
func (ws execWorkspace) run(t *testing.T, dryRun bool) evo.OperationCounts {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, StateDir: ws.state, Plain: true, DryRun: dryRun, Stdout: io.Discard, Stderr: io.Discard})
	out.Task("generate").Define(func(ctx context.Context) error {
		_, err := evo.Exec(ctx, evo.ExecSpec{
			Executable: "/bin/sh",
			Args:       []string{"-c", "printf generated > " + ws.output},
			Basis:      []evo.Fingerprint{evo.FSPath(ws.source)},
			Outputs:    []string{ws.output},
		})
		return err
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Conclusion().Metrics().Operations
}

func TestMetrics_ExecIdenticalOutputStopsPropagation(t *testing.T) {
	t.Parallel()
	ws := newExecWorkspace(t)
	if got, want := ws.run(t, false), (evo.OperationCounts{Executed: 1, Changed: 1}); got != want {
		t.Fatalf("first run Operations = %+v, want %+v", got, want)
	}
	ws.editSource(t, "v2")
	got := ws.run(t, false)
	if want := (evo.OperationCounts{Executed: 1, BasisDrift: 1, Unchanged: 1}); got != want {
		t.Fatalf("drifted run Operations = %+v, want %+v", got, want)
	}
	if got.PropagationStoppedRate() != 1 || got.ChangeRate() != 0 {
		t.Fatalf("PropagationStoppedRate = %v, ChangeRate = %v; want 1 and 0", got.PropagationStoppedRate(), got.ChangeRate())
	}
}

func TestMetrics_DryRunExecClaimsNoChange(t *testing.T) {
	t.Parallel()
	ws := newExecWorkspace(t)
	got := ws.run(t, true)
	if got.Changed != 0 || got.Unchanged != 0 {
		t.Fatalf("dry-run Operations = %+v: the command never ran, so it can report neither a changed nor an identical output", got)
	}
	if got.Executed != 1 {
		t.Fatalf("dry-run Executed = %d, want 1: the manifest could not prove the operation current", got.Executed)
	}
}
