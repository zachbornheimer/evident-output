package evo_test

import (
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// pipeline is the two-step Exec pipeline CONTRACT.md §3's cascade rule
// describes. Step B reads x.yaml and writes o.yaml (only x's first three
// bytes, so some edits to x leave o's bytes unchanged) and z.yaml (all of
// x, which nothing reads). Step A, ordered after B, reads o.yaml and writes
// y.yaml. A dryRun pipeline only plans its steps.
type pipeline struct {
	world  freshnessWorld
	dryRun bool
}

// pipelineRan is which steps ran their command during one Run.
type pipelineRan struct{ B, A bool }

func (p pipeline) run(t *testing.T) (pipelineRan, map[string]any) {
	t.Helper()
	w := p.world
	var ran pipelineRan
	run := executeContract(context.Background(), t,
		evo.Config{Name: "cascade", Output: evo.JSON, DryRun: p.dryRun, State: evo.State{Dir: w.state, AppID: "contract"}},
		func(_ context.Context, r *evo.Run) error {
			b := r.Task("B").Define(func(ctx context.Context) error {
				res, err := evo.Exec(ctx, evo.ExecSpec{
					Argv:    []string{"sh", "-c", "cut -c1-3 x.yaml > o.yaml && cp x.yaml z.yaml"},
					Dir:     w.dir,
					Inputs:  []string{w.path("x.yaml")},
					Outputs: []string{w.path("o.yaml"), w.path("z.yaml")},
				})
				ran.B = res.Ran
				return err
			})
			r.Task("A").After(b).Define(func(ctx context.Context) error {
				res, err := evo.Exec(ctx, evo.ExecSpec{
					Argv:    []string{"sh", "-c", "cp o.yaml y.yaml"},
					Dir:     w.dir,
					Inputs:  []string{w.path("o.yaml")},
					Outputs: []string{w.path("y.yaml")},
				})
				ran.A = res.Ran
				return err
			})
			return nil
		})
	if code := run.Result.ExitCode(); code != 0 {
		t.Fatalf("run exited %d:\n%s", code, run.Text())
	}
	return ran, runDocument(t, run.Stdout)
}

// TestExecPipelineCascadesFromInputsAndDriftedOutputs pins CONTRACT.md §3:
// a changed Input re-runs its step, a drifted Output re-runs its producer,
// and a consumer re-runs only when the bytes it reads changed.
func TestExecPipelineCascadesFromInputsAndDriftedOutputs(t *testing.T) {
	p := pipeline{world: newFreshnessWorld(t)}
	w := p.world
	steps := []struct {
		name   string
		change func()
		want   pipelineRan
	}{
		{"first run executes both", func() { w.write(t, "x.yaml", "abc-1\n") }, pipelineRan{B: true, A: true}},
		{"unchanged run executes neither", func() {}, pipelineRan{}},
		{"changed x cascades to A", func() { w.write(t, "x.yaml", "xyz-1\n") }, pipelineRan{B: true, A: true}},
		{"drifted z re-runs B alone", func() { w.write(t, "z.yaml", "hand edit\n") }, pipelineRan{B: true}},
		{"x change with identical o leaves A Satisfied", func() { w.write(t, "x.yaml", "xyz-2\n") }, pipelineRan{B: true}},
		{"drifted y re-runs A alone", func() { w.write(t, "y.yaml", "hand edit\n") }, pipelineRan{A: true}},
	}
	for _, step := range steps {
		step.change()
		ran, doc := p.run(t)
		if ran != step.want {
			t.Errorf("%s: ran B=%v A=%v, want B=%v A=%v", step.name, ran.B, ran.A, step.want.B, step.want.A)
		}
		assertStepOutcome(t, step.name, doc, "B", step.want.B)
		assertStepOutcome(t, step.name, doc, "A", step.want.A)
	}
	for name, want := range map[string]string{"o.yaml": "xyz\n", "z.yaml": "xyz-2\n", "y.yaml": "xyz\n"} {
		if got := w.read(t, name); got != want {
			t.Errorf("after the last Run %s = %q, want %q", name, got, want)
		}
	}
}

// assertStepOutcome checks the evo.run document gives Task name Succeeded
// when its command ran and Satisfied when it did not.
func assertStepOutcome(t *testing.T, at string, doc map[string]any, name string, ran bool) {
	t.Helper()
	want := evo.Satisfied
	if ran {
		want = evo.Succeeded
	}
	for _, task := range documentTasks(t, doc) {
		if task["name"] == name {
			if got := task["outcome"]; got != string(want) {
				t.Errorf("%s: task %s outcome = %v, want %q", at, name, got, want)
			}
			return
		}
	}
	t.Errorf("%s: no task %s in evo.run", at, name)
}

// TestDryRunPlansDownstreamOfPlannedOutputs pins CONTRACT.md §3 for a dry
// run: once x changed, B is planned, and A reads o.yaml, an Output of B's
// planned operation, so A is uncertain and planned too, never Satisfied.
// The real run that follows executes both, as the plan said.
func TestDryRunPlansDownstreamOfPlannedOutputs(t *testing.T) {
	applied := pipeline{world: newFreshnessWorld(t)}
	planned := pipeline{world: applied.world, dryRun: true}
	w := applied.world
	w.write(t, "x.yaml", "abc-1\n")
	applied.run(t)

	w.write(t, "x.yaml", "xyz-1\n")
	ran, doc := planned.run(t)
	if ran != (pipelineRan{}) {
		t.Errorf("dry run ran B=%v A=%v, want neither", ran.B, ran.A)
	}
	assertStepOutcome(t, "dry run", doc, "B", true)
	assertStepOutcome(t, "dry run", doc, "A", true)
	if got := w.read(t, "o.yaml"); got != "abc\n" {
		t.Errorf("dry run changed o.yaml to %q", got)
	}

	if ran, _ := applied.run(t); ran != (pipelineRan{B: true, A: true}) {
		t.Errorf("real run after the dry run ran B=%v A=%v, want both", ran.B, ran.A)
	}
	if ran, doc := planned.run(t); ran != (pipelineRan{}) {
		t.Errorf("dry run over current state ran B=%v A=%v", ran.B, ran.A)
	} else {
		assertStepOutcome(t, "dry run over current state", doc, "B", false)
		assertStepOutcome(t, "dry run over current state", doc, "A", false)
	}
}
