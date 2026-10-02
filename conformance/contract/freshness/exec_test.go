//go:build unix

package freshness_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

// stage is one shell-command Exec whose freshness the manifest decides.
type stage struct {
	name    string
	script  string
	basis   []string
	outputs []string
	env     map[string]string
}

// runStages executes stages in order as one run against stateDir and
// reports which stages actually ran their process. The run is closed so the
// manifest lock is released for the next run.
func runStages(t *testing.T, stateDir string, stages ...stage) map[string]bool {
	t.Helper()
	out, _ := harness.New(t, func(c *evo.Config) { c.StateDir = stateDir; c.AppID = "contract-freshness" })
	ran := map[string]bool{}
	seq := out.Sequence("stages")
	for _, st := range stages {
		task := seq.Task(st.name)
		for _, p := range st.basis {
			task.Basis(evo.File{Path: p})
		}
		for _, key := range slices.Sorted(maps.Keys(st.env)) {
			task.Basis(evo.Value("env."+key, st.env[key]))
		}
		task.Define(func(ctx context.Context) error {
			outputs := make(evo.Outputs, len(st.outputs))
			for i, p := range st.outputs {
				outputs[i] = evo.File{Path: p}
			}
			res, err := evo.Exec{
				Path: "sh", Args: []string{"-c", st.script}, Env: execEnv(st.env), Outputs: outputs,
			}.Run(ctx)
			ran[st.name] = res.Ran
			return err
		})
	}
	if err := seq.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	_ = out.Close()
	return ran
}

// execEnv is the child environment for explicit entries: the parent's
// environment plus them, so a set Env still finds sh's tools; nil inherits.
func execEnv(explicit map[string]string) []string {
	if len(explicit) == 0 {
		return nil
	}
	env := os.Environ()
	for _, key := range slices.Sorted(maps.Keys(explicit)) {
		env = append(env, key+"="+explicit[key])
	}
	return env
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

type generator struct {
	state, script, input, output string
}

func newGenerator(t *testing.T) generator {
	t.Helper()
	dir := t.TempDir()
	g := generator{
		state: filepath.Join(dir, "state"), script: filepath.Join(dir, "gen.sh"),
		input: filepath.Join(dir, "in.txt"), output: filepath.Join(dir, "out.txt"),
	}
	write(t, g.script, "tr -d ' ' < \"$1\"")
	write(t, g.input, "a b")
	return g
}

func (g generator) stage(env map[string]string) stage {
	return stage{
		name:   "generate",
		script: "sh " + g.script + " " + g.input + " > " + g.output,
		basis:  []string{g.script, g.input}, outputs: []string{g.output}, env: env,
	}
}

func TestC07_001_ChangedBasisReRunsAndUnchangedBasisDoesNot(t *testing.T) {
	g := newGenerator(t)
	if !runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("first run must execute")
	}
	if runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("unchanged Basis re-ran the process")
	}
	write(t, g.input, "a b c")
	if !runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("changed input Basis did not re-run")
	}
}

func TestC07_002_AmbientEnvironmentIsNotFingerprinted(t *testing.T) {
	g := newGenerator(t)
	runStages(t, g.state, g.stage(nil))
	t.Setenv("CONTRACT_UNRELATED", "changed")
	if runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("an unrelated environment variable invalidated the result")
	}
	runStages(t, g.state, g.stage(map[string]string{"MODE": "a"}))
	if !runStages(t, g.state, g.stage(map[string]string{"MODE": "b"}))["generate"] {
		t.Fatal("an explicit Env entry change did not invalidate the result")
	}
}

func TestC08_001_PreciseProvenanceTracksScriptAndInput(t *testing.T) {
	g := newGenerator(t)
	runStages(t, g.state, g.stage(nil))
	write(t, g.script, "tr -d ' ' < \"$1\" | tr a-z A-Z")
	if !runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("a changed script did not re-run the generator")
	}
	if runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("script and input unchanged, yet the generator re-ran")
	}
}

func TestC09_001_FreshnessFollowsChangedOutputsNotExecutedUpstream(t *testing.T) {
	dir := t.TempDir()
	state, src := filepath.Join(dir, "state"), filepath.Join(dir, "source.txt")
	norm, bin := filepath.Join(dir, "normalized.txt"), filepath.Join(dir, "schema.bin")
	write(t, src, "a b")
	stages := []stage{
		{name: "normalize", script: "tr -d ' ' < " + src + " > " + norm, basis: []string{src}, outputs: []string{norm}},
		{name: "compile", script: "cp " + norm + " " + bin, basis: []string{norm}, outputs: []string{bin}},
	}
	runStages(t, state, stages...)
	write(t, src, "a  b")
	ran := runStages(t, state, stages...)
	if !ran["normalize"] {
		t.Fatal("normalize must revalidate after its source changed")
	}
	if ran["compile"] {
		t.Fatal("compile re-ran although normalize produced an identical output")
	}
}

func TestC06_001_OpaqueCodeIsNeverSkippedFromTheManifest(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	runs := 0
	for range 2 {
		out, _ := harness.New(t, func(c *evo.Config) { c.StateDir = state; c.AppID = "contract-freshness" })
		task := out.Task("opaque")
		task.Define(func(context.Context) error { runs++; return nil })
		_ = task.Wait()
		_ = out.Finish()
		_ = out.Close()
	}
	if runs != 2 {
		t.Fatalf("opaque callback ran %d times across 2 runs, want 2", runs)
	}
}

func TestC10_001_MissingAndCorruptManifestsReExecuteSafely(t *testing.T) {
	g := newGenerator(t)
	if !runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("a missing manifest must run the work")
	}
	manifests, err := filepath.Glob(filepath.Join(g.state, "manifest-*.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %v, %v", manifests, err)
	}
	write(t, manifests[0], "{ not json")
	if !runStages(t, g.state, g.stage(nil))["generate"] {
		t.Fatal("a corrupt manifest was trusted as proof")
	}
}
