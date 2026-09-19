//go:build v06acceptance

// pipeline_test.go holds spec §64's multi-stage pipeline scenarios: a
// normalize stage (schema.xlsx -> schema.json) feeding a compile stage
// (schema.json + compile.py -> output.bin), proving the freshness graph's
// invalidation-stop and known-producer barrier end to end.
package exec_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// pipelineRunner routes each spawn to a per-executable-path scripted
// response and, when set, runs a side effect that writes the stage's real
// output content — testkit.ProcessRunner has no side-effect hook (its
// contract is "scripted stdout/stderr/exit and a gate"), so a scenario that
// needs a spawn to actually produce a file uses this small local fake
// instead, the same way internal/engine's own exec_test.go does.
type pipelineRunner struct {
	byPath map[string]func() (exitCode int)
}

func (r pipelineRunner) Run(ctx context.Context, cmd evo.ProcessCommand) (evo.ProcessOutcome, error) {
	fn, ok := r.byPath[cmd.Path]
	if !ok {
		return evo.ProcessOutcome{}, os.ErrNotExist
	}
	return evo.ProcessOutcome{ExitCode: fn()}, nil
}

// pipelineFixture is one §64 scenario's on-disk shape: normalize reads
// schema.xlsx and writes schema.json; compile reads schema.json and
// compile.py and writes output.bin. writeSchemaJSON/writeOutputBin let each
// run control exactly what content the stage "produces" so a test can
// choose whether that content actually changed.
type pipelineFixture struct {
	dir                        string
	normalizeTool, compileTool string
	schemaXlsx, compilePy      string
	schemaJSON, outputBin      string
}

func newPipelineFixture(t *testing.T) *pipelineFixture {
	t.Helper()
	dir := t.TempDir()
	f := &pipelineFixture{
		dir:           dir,
		normalizeTool: execFixture(t, dir, "normalize", "v1"),
		compileTool:   execFixture(t, dir, "compile", "v1"),
		schemaXlsx:    filepath.Join(dir, "schema.xlsx"),
		compilePy:     filepath.Join(dir, "compile.py"),
		schemaJSON:    filepath.Join(dir, "schema.json"),
		outputBin:     filepath.Join(dir, "output.bin"),
	}
	for _, seed := range []struct{ path, contents string }{
		{f.schemaXlsx, "xlsx-v1"},
		{f.compilePy, "compile-v1"},
		{f.schemaJSON, "json-v1"},
		{f.outputBin, "bin-v1"},
	} {
		if err := os.WriteFile(seed.path, []byte(seed.contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *pipelineFixture) normalizeSpec() evo.ExecSpec {
	return evo.ExecSpec{
		Executable: f.normalizeTool, Dir: f.dir,
		Basis:   []evo.Fingerprint{evo.FSPath(f.schemaXlsx)},
		Outputs: []string{"schema.json"},
	}
}

func (f *pipelineFixture) compileSpec() evo.ExecSpec {
	return evo.ExecSpec{
		Executable: f.compileTool, Dir: f.dir,
		Basis:   []evo.Fingerprint{evo.FSPath(f.schemaJSON), evo.FSPath(f.compilePy)},
		Outputs: []string{"output.bin"},
	}
}

// runPipeline runs both stages in one Run, compile sequenced After
// normalize — the realistic authoring shape spec §64 assumes (a Basis
// relationship alone never implies scheduling order: "no scheduler edge is
// inferred" is a statement about what the freshness graph does NOT do on
// its own, not license to skip declaring a real pipeline's real
// dependency). The freshness barrier's own concurrent-without-After
// behavior is covered directly at the engine level
// (TestExecFreshnessBarrierWaitsForProducerThenConsumesFinalOutput);  this
// suite proves the resulting invalidation-stop/cascade semantics end to
// end. schemaJSONFn/outputBinFn are each stage's spawn side effect.
func runPipeline(t *testing.T, state string, f *pipelineFixture, schemaJSONFn, outputBinFn func()) (normalizeCalls, compileCalls int) {
	t.Helper()
	normalizeCount, compileCount := 0, 0
	runner := pipelineRunner{byPath: map[string]func() int{
		f.normalizeTool: func() int {
			normalizeCount++
			if schemaJSONFn != nil {
				schemaJSONFn()
			}
			return 0
		},
		f.compileTool: func() int {
			compileCount++
			if outputBinFn != nil {
				outputBinFn()
			}
			return 0
		},
	}}
	out := evo.Init(evo.Config{Isolated: true, StateDir: state, ProcessRunner: runner})
	defer func() { _ = out.Close() }()

	out.Run(context.Background(), func(ctx context.Context) error {
		normalize := out.Task("normalize")
		normalize.Define(func(taskCtx context.Context) error { return evo.Exec(taskCtx, f.normalizeSpec()) })
		compile := out.Task("compile")
		compile.After(normalize)
		compile.Define(func(taskCtx context.Context) error { return evo.Exec(taskCtx, f.compileSpec()) })
		return nil
	})
	if err := out.Err(); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}
	return normalizeCount, compileCount
}

// TestV06PipelineUnchangedRunSpawnsNeitherStage proves both Define
// callbacks enter (Exec always evaluates) but neither stage spawns once a
// prior run already recorded matching Basis/Outputs for both.
func TestV06PipelineUnchangedRunSpawnsNeitherStage(t *testing.T) {
	state := t.TempDir()
	f := newPipelineFixture(t)
	writeSchema := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v1"), 0o644) }
	writeOutput := func() { _ = os.WriteFile(f.outputBin, []byte("bin-v1"), 0o644) }

	n1, c1 := runPipeline(t, state, f, writeSchema, writeOutput)
	if n1 != 1 || c1 != 1 {
		t.Fatalf("first run spawns = (%d,%d), want (1,1)", n1, c1)
	}

	n2, c2 := runPipeline(t, state, f, writeSchema, writeOutput)
	if n2 != 0 || c2 != 0 {
		t.Fatalf("unchanged rerun spawns = (%d,%d), want (0,0)", n2, c2)
	}
}

// TestV06PipelineSchemaChangeWithIdenticalNormalizedOutputStopsInvalidation
// proves spec §64's invalidation stop: schema.xlsx changing forces
// normalize to respawn (its own Basis changed), but when the resulting
// schema.json content is byte-identical to before, compile's Basis digest
// on schema.json is unchanged, so compile does not respawn.
func TestV06PipelineSchemaChangeWithIdenticalNormalizedOutputStopsInvalidation(t *testing.T) {
	state := t.TempDir()
	f := newPipelineFixture(t)
	writeSameSchema := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v1"), 0o644) }
	writeOutput := func() { _ = os.WriteFile(f.outputBin, []byte("bin-v1"), 0o644) }
	runPipeline(t, state, f, writeSameSchema, writeOutput)

	if err := os.WriteFile(f.schemaXlsx, []byte("xlsx-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, c := runPipeline(t, state, f, writeSameSchema, writeOutput)
	if n != 1 {
		t.Fatalf("normalize spawns = %d, want 1 (its own Basis changed)", n)
	}
	if c != 0 {
		t.Fatalf("compile spawns = %d, want 0 (invalidation must stop: schema.json content unchanged)", c)
	}
}

// TestV06PipelineSchemaChangeWithDifferentNormalizedOutputCascades proves
// the complement: when the re-normalized schema.json content actually
// differs, compile's Basis digest changes and it respawns too.
func TestV06PipelineSchemaChangeWithDifferentNormalizedOutputCascades(t *testing.T) {
	state := t.TempDir()
	f := newPipelineFixture(t)
	writeSchemaV1 := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v1"), 0o644) }
	writeOutput := func() { _ = os.WriteFile(f.outputBin, []byte("bin-v1"), 0o644) }
	runPipeline(t, state, f, writeSchemaV1, writeOutput)

	if err := os.WriteFile(f.schemaXlsx, []byte("xlsx-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSchemaV2 := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v2"), 0o644) }
	n, c := runPipeline(t, state, f, writeSchemaV2, writeOutput)
	if n != 1 || c != 1 {
		t.Fatalf("cascading change spawns = (%d,%d), want (1,1)", n, c)
	}
}

// TestV06PipelineCompilePyChangeRespawnsOnlyCompile proves a Basis change
// scoped to one stage never disturbs the other: compile.py is only in
// compile's Basis, so normalize stays current while compile respawns.
func TestV06PipelineCompilePyChangeRespawnsOnlyCompile(t *testing.T) {
	state := t.TempDir()
	f := newPipelineFixture(t)
	writeSchema := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v1"), 0o644) }
	writeOutput := func() { _ = os.WriteFile(f.outputBin, []byte("bin-v1"), 0o644) }
	runPipeline(t, state, f, writeSchema, writeOutput)

	if err := os.WriteFile(f.compilePy, []byte("compile-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, c := runPipeline(t, state, f, writeSchema, writeOutput)
	if n != 0 {
		t.Fatalf("normalize spawns = %d, want 0 (compile.py is not in its Basis)", n)
	}
	if c != 1 {
		t.Fatalf("compile spawns = %d, want 1 (compile.py is in its Basis)", c)
	}
}

// TestV06PipelineUnrelatedFileChangeSpawnsNeitherStage proves a change to a
// file neither stage declares in Basis or Outputs never disturbs either.
// Application-fingerprint change with/without evo.App() in Basis is covered
// by TestV06PipelineAppFingerprintChangeWithPreciseBasisSpawnsNeitherStage
// and TestV06ExecAppFingerprintChangeWithAppBasisRespawns.
func TestV06PipelineUnrelatedFileChangeSpawnsNeitherStage(t *testing.T) {
	state := t.TempDir()
	f := newPipelineFixture(t)
	writeSchema := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v1"), 0o644) }
	writeOutput := func() { _ = os.WriteFile(f.outputBin, []byte("bin-v1"), 0o644) }
	runPipeline(t, state, f, writeSchema, writeOutput)

	unrelated := filepath.Join(f.dir, "README.md")
	if err := os.WriteFile(unrelated, []byte("unrelated change"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, c := runPipeline(t, state, f, writeSchema, writeOutput)
	if n != 0 || c != 0 {
		t.Fatalf("unrelated file change spawns = (%d,%d), want (0,0)", n, c)
	}
}
