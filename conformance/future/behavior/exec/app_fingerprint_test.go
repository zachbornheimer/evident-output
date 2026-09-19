//go:build v06acceptance

package exec_test

import (
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestV06PipelineAppFingerprintChangeWithPreciseBasisSpawnsNeitherStage
// proves spec §64 / §19-20: when neither stage declares evo.App() in Basis,
// changing the application fingerprint must not respawn either stage and
// must leave Outputs untouched — precise Basis (script/input paths) owns
// freshness, not app identity.
func TestV06PipelineAppFingerprintChangeWithPreciseBasisSpawnsNeitherStage(t *testing.T) {
	state := t.TempDir()
	f := newPipelineFixture(t)
	writeSchema := func() { _ = os.WriteFile(f.schemaJSON, []byte("json-v1"), 0o644) }
	writeOutput := func() { _ = os.WriteFile(f.outputBin, []byte("bin-v1"), 0o644) }

	testkit.WithFakeApp("app-v1", func() {
		n, c := runPipeline(t, state, f, writeSchema, writeOutput)
		if n != 1 || c != 1 {
			t.Fatalf("first run spawns = (%d,%d), want (1,1)", n, c)
		}
	})

	before, err := os.ReadFile(f.outputBin)
	if err != nil {
		t.Fatal(err)
	}

	testkit.WithFakeApp("app-v2", func() {
		n, c := runPipeline(t, state, f, writeSchema, writeOutput)
		if n != 0 || c != 0 {
			t.Fatalf("app fingerprint change with precise Basis spawns = (%d,%d), want (0,0)", n, c)
		}
	})

	after, err := os.ReadFile(f.outputBin)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("output.bin changed from %q to %q; precise-Basis skip must leave Outputs untouched", before, after)
	}
}

// TestV06ExecAppFingerprintChangeWithAppBasisRespawns proves the
// conservative complement: an explicit evo.App() Basis entry makes an
// application-fingerprint change force a respawn.
func TestV06ExecAppFingerprintChangeWithAppBasisRespawns(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	input := filepath.Join(dir, "input.txt")
	outPath := filepath.Join(dir, "out.txt")
	for _, seed := range []struct{ path, contents string }{
		{input, "in-v1"},
		{outPath, "built"},
	} {
		if err := os.WriteFile(seed.path, []byte(seed.contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	spec := evo.ExecSpec{
		Executable: tool, Dir: dir,
		Basis:   []evo.Fingerprint{evo.FSPath(input), evo.App()},
		Outputs: []string{"out.txt"},
	}

	testkit.WithFakeApp("app-v1", func() {
		runner := testkit.NewProcessRunner()
		runner.Script(tool, testkit.ScriptedProcess{ExitCode: 0})
		out := isolated(state, runner)
		if err := runExecTask(out, "build", spec); err != nil {
			t.Fatalf("first run: %v", err)
		}
		_ = out.Close()
	})

	testkit.WithFakeApp("app-v2", func() {
		runner := testkit.NewProcessRunner()
		runner.Script(tool, testkit.ScriptedProcess{ExitCode: 0})
		out := isolated(state, runner)
		t.Cleanup(func() { _ = out.Close() })
		if err := runExecTask(out, "build", spec); err != nil {
			t.Fatalf("second run: %v", err)
		}
		if calls := runner.Calls(); len(calls) != 1 {
			t.Fatalf("app fingerprint change with App() in Basis spawned %d times, want 1", len(calls))
		}
	})
}
