package evo_test

import (
	"bytes"
	"context"
	"io"
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

// run reconciles both files once, rendering Verbose plain output.
func (ws provenanceWorkspace) run(t *testing.T) (evo.RunMetrics, evo.TaskSnapshot, string) {
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
	return out.Conclusion().Metrics(), config.Snapshot(), buf.String()
}

func TestMetrics_TrackedOperationsTallyManifestHitsDriftAndPropagation(t *testing.T) {
	t.Parallel()
	ws := newProvenanceWorkspace(t)

	first, _, _ := ws.run(t)
	if want := (evo.OperationCounts{Executed: 2, Changed: 2}); first.Operations != want {
		t.Fatalf("first run Operations = %+v, want %+v", first.Operations, want)
	}
	second, _, _ := ws.run(t)
	if second.Operations != (evo.OperationCounts{Current: 2}) || second.Operations.HitRate() != 1 {
		t.Fatalf("second run Operations = %+v, want both current (hit rate 1)", second.Operations)
	}

	ws.editSource(t, "v2")
	third, config, rendered := ws.run(t)
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
