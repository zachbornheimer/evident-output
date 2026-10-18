package evo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestFileInputsResolveAgainstWorkspace pins CONTRACT.md §8: a relative
// Input resolves against the workspace the Run started in, so a body that
// changes directory neither moves the Input nor hides an edit to it.
func TestFileInputsResolveAgainstWorkspace(t *testing.T) {
	w := newFreshnessWorld(t)
	elsewhere := t.TempDir()
	t.Chdir(w.dir)
	run := func() contractRun {
		if err := os.Chdir(w.dir); err != nil {
			t.Fatal(err)
		}
		return w.run(t, func(ctx context.Context) error {
			if err := os.Chdir(elsewhere); err != nil {
				return err
			}
			return evo.File(ctx, evo.FileSpec{Path: "out.txt", Contents: []byte("generated\n"), Inputs: []string{"in.txt"}})
		})
	}
	w.write(t, "in.txt", "one\n")
	run()
	if err := os.WriteFile(filepath.Join(elsewhere, "in.txt"), []byte("decoy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again := run(); strings.Contains(again.Text(), "[changed]") {
		t.Errorf("an unchanged workspace Input re-ran File:\n%s", again.Text())
	}
	w.write(t, "in.txt", "two\n")
	if edited := run(); !strings.Contains(edited.Text(), "[changed]") {
		t.Errorf("an edited workspace Input left File Satisfied:\n%s", edited.Text())
	}
}

// TestDryRunPlansFileDownstreamOfPlannedExec pins CONTRACT.md §3 for a
// relative-Input File: x -> o (File) -> z (Exec) -> y (File reading z.txt).
// Once x changed, the dry run plans y too, since z is planned.
func TestDryRunPlansFileDownstreamOfPlannedExec(t *testing.T) {
	w := newFreshnessWorld(t)
	t.Chdir(w.dir)
	pipe := func(dry bool) map[string]any {
		run := executeContract(context.Background(), t,
			evo.Config{Name: "cascade", Output: evo.JSON, DryRun: dry, State: evo.State{Dir: w.state, AppID: "contract"}},
			func(_ context.Context, r *evo.Run) error {
				o := r.Task("o").Define(func(ctx context.Context) error {
					return evo.File(ctx, evo.FileSpec{Path: "o.txt", Contents: []byte("o\n"), Inputs: []string{"x.txt"}})
				})
				z := r.Task("z").After(o).Define(func(ctx context.Context) error {
					_, err := evo.Exec(ctx, evo.ExecSpec{
						Argv: []string{"sh", "-c", "cp o.txt z.txt"}, Dir: w.dir,
						Inputs: []string{w.path("o.txt")}, Outputs: []string{w.path("z.txt")},
					})
					return err
				})
				r.Task("y").After(z).Define(func(ctx context.Context) error {
					return evo.File(ctx, evo.FileSpec{Path: "y.txt", Contents: []byte("y\n"), Inputs: []string{"z.txt"}})
				})
				return nil
			})
		if code := run.Result.ExitCode(); code != 0 {
			t.Fatalf("run exited %d:\n%s", code, run.Text())
		}
		return runDocument(t, run.Stdout)
	}
	w.write(t, "x.txt", "one\n")
	pipe(false)
	w.write(t, "x.txt", "two\n")
	doc := pipe(true)
	for _, name := range []string{"o", "z", "y"} {
		for _, task := range documentTasks(t, doc) {
			if task["name"] == name && task["outcome"] == string(evo.Satisfied) {
				t.Errorf("dry run reads Task %s as Satisfied, want planned", name)
			}
		}
	}
}
