// Package tests22_test binds contract §22 behavior proofs to the public evo
// API. The pipeline fixture is a normalize stage (schema.xlsx to
// schema.json) feeding a compile stage (schema.json and compile.py to
// output.bin), each one evo.Exec with precise Basis and declared Outputs.
package tests22_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	normalizedV1 = "json-v1"
	compiledV1   = "bin-v1"
)

// spawnCounter is a ProcessRunner that counts spawns per executable and runs
// an optional side effect that writes the stage's real output.
type spawnCounter struct {
	effects map[string]func()
	spawns  map[string]int
}

func (r *spawnCounter) Run(_ context.Context, cmd evo.ProcessCommand) (evo.ProcessOutcome, error) {
	r.spawns[cmd.Path]++
	if effect := r.effects[cmd.Path]; effect != nil {
		effect()
	}
	return evo.ProcessOutcome{}, nil
}

type pipeline struct {
	dir                        string
	normalizeTool, compileTool string
	schemaXlsx, compilePy      string
	schemaJSON, outputBin      string
	unrelated                  string
	state                      string
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func newPipeline(t *testing.T) *pipeline {
	t.Helper()
	dir := t.TempDir()
	p := &pipeline{
		dir:           dir,
		normalizeTool: filepath.Join(dir, "normalize"),
		compileTool:   filepath.Join(dir, "compile"),
		schemaXlsx:    filepath.Join(dir, "schema.xlsx"),
		compilePy:     filepath.Join(dir, "compile.py"),
		schemaJSON:    filepath.Join(dir, "schema.json"),
		outputBin:     filepath.Join(dir, "output.bin"),
		unrelated:     filepath.Join(dir, "notes.txt"),
		state:         t.TempDir(),
	}
	for path, contents := range map[string]string{
		p.normalizeTool: "v1", p.compileTool: "v1", p.schemaXlsx: "xlsx-v1",
		p.compilePy: "compile-v1", p.schemaJSON: normalizedV1, p.outputBin: compiledV1,
		p.unrelated: "notes-v1",
	} {
		write(t, path, contents)
	}
	return p
}

func (p *pipeline) normalizeSpec() evo.ExecSpec {
	return evo.ExecSpec{
		Executable: p.normalizeTool, Dir: p.dir,
		Outputs: []string{"schema.json"},
		Basis:   []evo.Fingerprint{evo.FSPath(p.schemaXlsx)},
	}
}

func (p *pipeline) compileSpec() evo.ExecSpec {
	return evo.ExecSpec{
		Executable: p.compileTool, Dir: p.dir,
		Outputs: []string{"output.bin"},
		Basis:   []evo.Fingerprint{evo.FSPath(p.schemaJSON), evo.FSPath(p.compilePy)},
	}
}

// run executes both stages once (compile declared After normalize) and
// returns how many times each stage spawned. normalizedOutput is the
// content the normalize spawn writes to schema.json.
func (p *pipeline) run(t *testing.T, normalizedOutput string) (normalizeSpawns, compileSpawns int) {
	t.Helper()
	runner := &spawnCounter{
		spawns: map[string]int{},
		effects: map[string]func(){
			p.normalizeTool: func() { write(t, p.schemaJSON, normalizedOutput) },
			p.compileTool:   func() { write(t, p.outputBin, compiledV1) },
		},
	}
	out := evo.Init(evo.Config{
		Isolated: true, StateDir: p.state, ProcessRunner: runner,
		Stdout: io.Discard, Stderr: io.Discard,
	})
	stage := func(name string, spec evo.ExecSpec, after ...*evo.TaskHandle) *evo.TaskHandle {
		task := out.Task(name)
		for _, pred := range after {
			task.After(pred)
		}
		return task.Define(func(ctx context.Context) error {
			_, err := evo.Exec(ctx, spec)
			return err
		})
	}
	normalize := stage("normalize", p.normalizeSpec())
	stage("compile", p.compileSpec(), normalize)
	_ = out.Finish()
	if err := out.Close(); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}
	return runner.spawns[p.normalizeTool], runner.spawns[p.compileTool]
}

// seeded runs the pipeline once so the manifest records both stages.
func (p *pipeline) seeded(t *testing.T) {
	t.Helper()
	if n, c := p.run(t, normalizedV1); n != 1 || c != 1 {
		t.Fatalf("seed run spawns = (%d,%d), want (1,1)", n, c)
	}
}

func TestC22_014_ChangedBasisInvalidatesAndAnUnchangedRunSpawnsNothing(t *testing.T) {
	p := newPipeline(t)
	p.seeded(t)
	if n, c := p.run(t, normalizedV1); n != 0 || c != 0 {
		t.Fatalf("unchanged rerun spawns = (%d,%d), want (0,0)", n, c)
	}
	write(t, p.schemaXlsx, "xlsx-v2")
	if n, _ := p.run(t, normalizedV1); n != 1 {
		t.Fatalf("normalize spawns = %d after its Basis changed, want 1", n)
	}
}

func TestC22_015_OnlyTheStageWhoseScriptOrInputChangedRespawns(t *testing.T) {
	p := newPipeline(t)
	p.seeded(t)
	write(t, p.unrelated, "notes-v2")
	if n, c := p.run(t, normalizedV1); n != 0 || c != 0 {
		t.Fatalf("unrelated file change spawns = (%d,%d), want (0,0)", n, c)
	}
	write(t, p.compilePy, "compile-v2")
	if n, c := p.run(t, normalizedV1); n != 0 || c != 1 {
		t.Fatalf("compile script change spawns = (%d,%d), want only compile", n, c)
	}
}

func TestC22_017_UnchangedUpstreamOutputStopsDownstreamInvalidation(t *testing.T) {
	p := newPipeline(t)
	p.seeded(t)
	write(t, p.schemaXlsx, "xlsx-v2")
	n, c := p.run(t, normalizedV1)
	if n != 1 {
		t.Fatalf("normalize spawns = %d, want 1 (its own Basis changed)", n)
	}
	if c != 0 {
		t.Fatalf("compile spawns = %d, want 0: schema.json content is unchanged", c)
	}
	write(t, p.schemaXlsx, "xlsx-v3")
	if n, c := p.run(t, "json-v2"); n != 1 || c != 1 {
		t.Fatalf("changed upstream output spawns = (%d,%d), want (1,1): the change must cascade", n, c)
	}
}
