package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The §39 provenance fixture: one config file generated from a source
// (its Basis) and one static README, reconciled by evo.File across three
// runs that share one manifest. Run two proves both current; run three
// follows a source edit that leaves the generated config byte-identical, so
// the Basis drift revalidates it and propagation stops at identical output.

type provenanceWorkspace struct {
	state, source, config, readme string
}

func newProvenanceWorkspace(t *testing.T) provenanceWorkspace {
	t.Helper()
	dir := t.TempDir()
	ws := provenanceWorkspace{
		state:  t.TempDir(),
		source: filepath.Join(dir, "config.src"),
		config: filepath.Join(dir, "config.json"),
		readme: filepath.Join(dir, "README"),
	}
	ws.editSource(t, "v1")
	return ws
}

func (ws provenanceWorkspace) editSource(t *testing.T, text string) {
	t.Helper()
	if err := os.WriteFile(ws.source, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// provenanceRun is one reconcile: its finished Output, the config Task's
// snapshot, and the Verbose plain rendering.
type provenanceRun struct {
	out      *evo.Output
	config   evo.TaskSnapshot
	rendered string
}

func (r provenanceRun) metrics() evo.RunMetrics { return r.out.Conclusion().Metrics() }

// run reconciles both files once, rendering Verbose plain output.
func (ws provenanceWorkspace) run(t *testing.T) provenanceRun {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, StateDir: ws.state, Plain: true, Color: evo.ColorNever,
		Verbosity: evo.VerbosityVerbose, Stdout: &buf, Stderr: io.Discard,
	})
	config := out.Task("config").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: ws.config, Contents: []byte(`{"ok":true}`), Basis: []evo.Fingerprint{evo.FSPath(ws.source)}})
	})
	out.Task("readme").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: ws.readme, Contents: []byte("docs\n")})
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Close releases the manifest's cross-process lock for the next run.
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return provenanceRun{out: out, config: config.Snapshot(), rendered: buf.String()}
}

func TestMetrics_TrackedOperationsTallyManifestHitsDriftAndPropagation(t *testing.T) {
	t.Parallel()
	ws := newProvenanceWorkspace(t)

	first := ws.run(t).metrics()
	if want := (evo.OperationCounts{Executed: 2, Changed: 2}); first.Operations != want {
		t.Fatalf("first run Operations = %+v, want %+v", first.Operations, want)
	}
	second := ws.run(t).metrics()
	if second.Operations != (evo.OperationCounts{Current: 2}) || second.Operations.HitRate() != 1 {
		t.Fatalf("second run Operations = %+v, want both current (hit rate 1)", second.Operations)
	}

	ws.editSource(t, "v2")
	run := ws.run(t)
	third, config, rendered := run.metrics(), run.config, run.rendered
	want := evo.OperationCounts{Current: 1, Executed: 1, BasisDrift: 1, Unchanged: 1}
	if third.Operations != want {
		t.Fatalf("third run Operations = %+v, want %+v", third.Operations, want)
	}
	rates := map[string][2]float64{
		"HitRate":                {third.Operations.HitRate(), 0.5},
		"BasisInvalidationRate":  {third.Operations.BasisInvalidationRate(), 0.5},
		"PropagationStoppedRate": {third.Operations.PropagationStoppedRate(), 1},
		"CallbackEntryRate":      {third.CallbackEntryRate(), 1},
	}
	for name, r := range rates {
		if r[0] != r[1] {
			t.Errorf("%s = %v, want %v", name, r[0], r[1])
		}
	}
	timing := config.Timing
	if timing.Definition.Entries != 1 || timing.Provenance.Entries == 0 || timing.TrackedState.Entries != 1 {
		t.Errorf("config phases = definition %+v, provenance %+v, tracked state %+v; want one definition, provenance checks, one live inspection",
			timing.Definition, timing.Provenance, timing.TrackedState)
	}
	for _, clause := range []string{"1 of 2 operations current", "1 basis changed", "1 identical outputs"} {
		if !strings.Contains(rendered, clause) {
			t.Errorf("verbose timing line lacks %q:\n%s", clause, rendered)
		}
	}
}

// A JSON consumer finds which Task drove a low manifest hit rate from each
// Task's own operation counts and phase entries, without the Go API.
func TestMetrics_FinalJSONAttributesOperationsAndPhaseEntriesToEachTask(t *testing.T) {
	t.Parallel()
	ws := newProvenanceWorkspace(t)
	ws.run(t)
	ws.editSource(t, "v2")
	run := ws.run(t)

	var doc struct {
		Data struct {
			Tasks []struct {
				Name            string           `json:"name"`
				OperationCounts map[string]int   `json:"operation_counts"`
				Timing          map[string]int64 `json:"timing"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(machineDocument(t, run.out)), &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]int{
		"config": {"current": 0, "executed": 1, "basis_drift": 1, "changed": 0, "unchanged": 1},
		"readme": {"current": 1, "executed": 0, "basis_drift": 0, "changed": 0, "unchanged": 0},
	}
	for _, task := range doc.Data.Tasks {
		if !maps.Equal(task.OperationCounts, want[task.Name]) {
			t.Errorf("%s operation_counts = %v, want %v", task.Name, task.OperationCounts, want[task.Name])
		}
		if got := task.Timing["definition_entries"]; got != 1 {
			t.Errorf("%s timing.definition_entries = %d, want 1", task.Name, got)
		}
	}
	if got, want := doc.Data.Tasks[0].Timing["tracked_state_entries"], int64(run.config.Timing.TrackedState.Entries); got != want {
		t.Errorf("config timing.tracked_state_entries = %d, want %d (the Go snapshot's count)", got, want)
	}
}
