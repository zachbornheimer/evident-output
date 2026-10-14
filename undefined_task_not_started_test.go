package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// annotatedUndefined is one thing a caller can hang on a Task that has no
// Define, and the CONTRACT.md §4 promise that none of it settles the Task.
type annotatedUndefined struct {
	name     string
	annotate func(*evo.Task) *evo.Task
}

var undefinedAnnotations = []annotatedUndefined{
	{"bare", func(t *evo.Task) *evo.Task { return t }},
	{"fact", func(t *evo.Task) *evo.Task { return t.Fact("k", "v") }},
	{"summary", func(t *evo.Task) *evo.Task { return t.Summary("said something") }},
	{"problem", func(t *evo.Task) *evo.Task { return t.Problem("heads up") }},
	{"progress", func(t *evo.Task) *evo.Task { return t.Progress(3, 3) }},
	{"verify", func(t *evo.Task) *evo.Task {
		return t.Verify(func(context.Context) (bool, error) { return true, nil })
	}},
}

// undefinedRun declares an annotated Task that is never Defined and a Task
// After it that is.
func undefinedRun(t *testing.T, annotation annotatedUndefined, output evo.Output) contractRun {
	t.Helper()
	return executeContract(context.Background(), t, evo.Config{Name: "undefined", Output: output},
		func(_ context.Context, r *evo.Run) error {
			stalled := annotation.annotate(r.Task("stalled"))
			r.Task("after").After(stalled).Define(func(context.Context) error { return nil })
			return nil
		})
}

// TestUndefinedTaskEndsNotStarted pins CONTRACT.md §4: a Task with no Define
// never returns, so it ends NotStarted at Finish whatever Facts, Problems,
// Progress or Verify it carries, its dependent names it in that state, and
// the run reads partial instead of a plain success.
func TestUndefinedTaskEndsNotStarted(t *testing.T) {
	for _, annotation := range undefinedAnnotations {
		t.Run(annotation.name, func(t *testing.T) {
			tasks := map[string]map[string]any{}
			for _, task := range documentTasks(t, runDocument(t, undefinedRun(t, annotation, evo.JSON).Stdout)) {
				tasks[task["name"].(string)] = task
			}
			stalled := tasks["stalled"]
			if got := stalled["state"]; got != "not_started" {
				t.Errorf("stalled state = %v, want not_started", got)
			}
			if got := stalled["definition_executed"]; got != false {
				t.Errorf("stalled definition_executed = %v, want false", got)
			}
			if got, _ := tasks["after"]["summary"].(string); got != "not started — stalled not started" {
				t.Errorf("after summary = %q, want %q", got, "not started — stalled not started")
			}
			plain := undefinedRun(t, annotation, evo.Plain)
			if got, want := plain.Result.ExitCode(), 0; got != want {
				t.Errorf("exit code = %d, want %d: partial qualifies a Succeeded run and never replaces it", got, want)
			}
			if !strings.Contains(plain.Stderr, "[succeeded · partial]  undefined") {
				t.Errorf("Plain band is not [succeeded · partial]:\n%s", plain.Stderr)
			}
		})
	}
}

// TestUndefinedTaskPlainGolden pins what a reader sees: the undefined Task's
// row, the dependent naming it as "not started", and a Next step that names
// the Task to give a Define.
func TestUndefinedTaskPlainGolden(t *testing.T) {
	ok := func(context.Context) error { return nil }
	run := executeContract(context.Background(), t, evo.Config{Name: "undefined", Output: evo.Plain},
		func(_ context.Context, r *evo.Run) error {
			audit := r.Task("audit").Problem("audit is advisory", evo.On("audit.cfg"))
			r.Task("publish").After(audit).Define(ok)
			r.Task("lint")
			return nil
		})
	want := "- audit    not started\n" +
		"   └─ audit.cfg  audit is advisory\n" +
		"- publish  not started — audit not started\n" +
		"- lint     not started\n\n" +
		"[succeeded · partial]  undefined\n" +
		"→  give Task \"audit\" (and 1 more) a Define callback that returns nil, evo.Fail, evo.Refuse or evo.Exclude\n"
	if run.Stderr != want {
		t.Errorf("Plain output =\n%s\nwant\n%s", run.Stderr, want)
	}
}
