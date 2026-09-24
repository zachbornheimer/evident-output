package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
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

// run executes one generator Exec and returns its operation tallies.
func (ws execWorkspace) run(t *testing.T, dryRun bool) evo.OperationCounts {
	t.Helper()
	return ws.runWith(t, evo.Config{DryRun: dryRun, Stdout: io.Discard}).Conclusion().Metrics().Operations
}

// runWith executes one generator Exec whose output ignores its Basis
// content, under cfg's mode and output stream.
func (ws execWorkspace) runWith(t *testing.T, cfg evo.Config) *evo.Output {
	t.Helper()
	cfg.Isolated, cfg.StateDir, cfg.Plain, cfg.Stderr = true, ws.state, true, io.Discard
	out := evo.Init(cfg)
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
	return out
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

// operationFinishedPayload runs one Exec streaming JSONL and returns its
// operation.finished payload.
func (ws execWorkspace) operationFinishedPayload(t *testing.T, dryRun bool) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	ws.runWith(t, evo.Config{DryRun: dryRun, Stdout: &buf, Format: evo.FormatJSONL})
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var ev struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		if ev.Type == "operation.finished" {
			return ev.Payload
		}
	}
	t.Fatalf("no operation.finished event in:\n%s", buf.String())
	return nil
}

// The operation.finished payload has two typed shapes (schema/event.v2.json):
// an observed one that reports changed, and a dry run's planned one that
// never ran, so it reports no outcome at all.
func TestMetrics_ExecOperationFinishedPayloadsConformToTheirTypedShapes(t *testing.T) {
	t.Parallel()
	schema, err := os.ReadFile("schema/event.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	ws := newExecWorkspace(t)
	shapes := []struct {
		def             string
		dryRun          bool
		present, absent string
	}{
		{def: "operationPlannedPayload", dryRun: true, present: "planned", absent: "changed"},
		{def: "operationObservedPayload", dryRun: false, present: "changed", absent: "planned"},
	}
	for _, shape := range shapes {
		payload := ws.operationFinishedPayload(t, shape.dryRun)
		if err := wireschema.ValidateDef(schema, mustJSON(t, payload), shape.def); err != nil {
			t.Errorf("dry run %v: payload %v does not conform to $defs/%s: %v", shape.dryRun, payload, shape.def, err)
		}
		if _, ok := payload[shape.present]; !ok {
			t.Errorf("dry run %v: payload %v lacks %q", shape.dryRun, payload, shape.present)
		}
		if _, ok := payload[shape.absent]; ok {
			t.Errorf("dry run %v: payload %v carries %q; the shapes are exclusive", shape.dryRun, payload, shape.absent)
		}
	}
}
