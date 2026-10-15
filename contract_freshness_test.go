package evo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// freshnessWorld is one directory of inputs and outputs plus the state dir
// that remembers them across Runs, the way repeated CLI invocations do.
type freshnessWorld struct {
	dir   string
	state string
}

func newFreshnessWorld(t *testing.T) freshnessWorld {
	t.Helper()
	return freshnessWorld{dir: t.TempDir(), state: t.TempDir()}
}

func (w freshnessWorld) path(name string) string { return filepath.Join(w.dir, name) }

func (w freshnessWorld) write(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.WriteFile(w.path(name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func (w freshnessWorld) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(w.path(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// run executes body as one Run against w's state dir, in Plain.
func (w freshnessWorld) run(t *testing.T, body func(ctx context.Context) error) contractRun {
	t.Helper()
	run := executeContract(context.Background(), t,
		evo.Config{Name: "freshness", Output: evo.Plain, State: evo.State{Dir: w.state, AppID: "contract"}},
		func(_ context.Context, r *evo.Run) error {
			r.Task("generate").Define(body)
			return nil
		})
	if code := run.Result.ExitCode(); code != 0 {
		t.Fatalf("run exited %d:\n%s", code, run.Text())
	}
	return run
}

// freshnessPass is one Run after a change to the world, and whether the
// operation must run again.
type freshnessPass struct {
	name   string
	input  string
	value  string
	reruns bool
}

var freshnessPasses = []freshnessPass{
	{"first run", "one\n", "a", true},
	{"nothing changed", "one\n", "a", false},
	{"Input changed", "two\n", "a", true},
	{"Value changed", "two\n", "b", true},
	{"nothing changed again", "two\n", "b", false},
}

// TestExecRerunsWhenInputOrValueChanges pins CONTRACT.md §3: Exec
// fingerprints its Inputs and Values, so a changed Input byte or Value runs
// the command again, and an unchanged world leaves it Satisfied.
func TestExecRerunsWhenInputOrValueChanges(t *testing.T) {
	w := newFreshnessWorld(t)
	in, out := w.path("in.txt"), w.path("out.txt")
	for _, pass := range freshnessPasses {
		w.write(t, "in.txt", pass.input)
		var ran bool
		w.run(t, func(ctx context.Context) error {
			res, err := evo.Exec(ctx, evo.ExecSpec{
				Argv:    []string{"cp", in, out},
				Inputs:  []string{in},
				Values:  map[string]string{"variant": pass.value},
				Outputs: []string{out},
			})
			ran = res.Ran
			return err
		})
		if ran != pass.reruns {
			t.Errorf("%s: ExecResult.Ran = %v, want %v", pass.name, ran, pass.reruns)
		}
		if got := w.read(t, "out.txt"); got != pass.input {
			t.Errorf("%s: out.txt = %q, want %q (the Input's bytes)", pass.name, got, pass.input)
		}
	}
}

// TestFileRerunsWhenInputOrValueChanges pins CONTRACT.md §3: File
// fingerprints its Inputs and Values, so a changed Input byte or Value
// commits the file again even when Contents is unchanged, and an unchanged
// world leaves it Satisfied with no [changed] row.
func TestFileRerunsWhenInputOrValueChanges(t *testing.T) {
	w := newFreshnessWorld(t)
	in, out := w.path("in.txt"), w.path("out.txt")
	for _, pass := range freshnessPasses {
		w.write(t, "in.txt", pass.input)
		run := w.run(t, func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{
				Path:     out,
				Contents: []byte("generated\n"),
				Inputs:   []string{in},
				Values:   map[string]string{"variant": pass.value},
			})
		})
		if rewrote := strings.Contains(run.Text(), "[changed]"); rewrote != pass.reruns {
			t.Errorf("%s: [changed] row present = %v, want %v:\n%s", pass.name, rewrote, pass.reruns, run.Text())
		}
	}
}
