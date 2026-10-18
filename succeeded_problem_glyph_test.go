package evo_test

import (
	"context"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestSucceededWithProblemUsesDistinctGlyph pins CONTRACT section 4: a Task
// that returned nil with a Problem recorded is Succeeded, and its row draws
// the attention glyph `!`, because `✓ docker daemon  cannot connect to docker
// socket` reads as success beside its own Problem. A Succeeded row with no
// Problem keeps `✓`.
func TestSucceededWithProblemUsesDistinctGlyph(t *testing.T) {
	for _, mode := range []evo.Output{evo.Human, evo.Plain} {
		t.Run(outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: "probe", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					r.Task("docker daemon").Problem("cannot connect to docker socket").
						Define(func(context.Context) error { return nil })
					r.Task("lint").Define(func(context.Context) error { return nil })
					return nil
				})
			if code := run.Result.ExitCode(); code != 0 {
				t.Fatalf("a Problem on a Task that returned nil must stay exit 0, got %d", code)
			}
			rows := rowsOf(run.Text())
			if row := rows["docker daemon"]; !strings.HasPrefix(row, "! docker daemon") {
				t.Errorf("Succeeded Task with a Problem draws %q, want a row led by `!`:\n%s", row, run.Text())
			}
			if row := rows["lint"]; !strings.HasPrefix(row, "✓ lint") {
				t.Errorf("Succeeded Task without a Problem draws %q, want a row led by `✓`:\n%s", row, run.Text())
			}
		})
	}
	contract, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contract), "| `!` ") {
		t.Error("CONTRACT.md section 4 does not name the `!` glyph for a Succeeded Task with a Problem")
	}
}

// rowsOf maps each row's Task name to its line, for a name the row starts with
// after its glyph.
func rowsOf(text string) map[string]string {
	rows := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		if _, rest, ok := strings.Cut(line, " "); ok {
			for _, name := range []string{"docker daemon", "lint"} {
				if strings.HasPrefix(rest, name) {
					rows[name] = line
				}
			}
		}
	}
	return rows
}
