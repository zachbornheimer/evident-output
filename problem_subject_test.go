package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// problemSubjectRun is one Task that records count Problems, each On("job").
func problemSubjectRun(t *testing.T, count int) (human string, doc map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	out := initRun(Config{Name: "warn", Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	task := out.Task("check jobs")
	task.Define(func(context.Context) error {
		for range count {
			task.Problem("x", On("job"))
		}
		return nil
	})
	_ = out.finish()
	var js bytes.Buffer
	if err := writeJSON(&js, Result{Conclusion: out.conclusion()}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	_ = out.close()
	if err := json.Unmarshal(js.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, js.String())
	}
	return buf.String(), doc
}

// TestProblem_OnSubjectReachesEveryProjection pins E-109: Problem("x",
// On("job")) names its subject in the human tree and carries it in the
// run.v3 task entry, however many Problems the Task records.
func TestProblem_OnSubjectReachesEveryProjection(t *testing.T) {
	for _, count := range []int{1, 2} {
		human, doc := problemSubjectRun(t, count)
		if got := strings.Count(human, "─ job\n"); got != count {
			t.Errorf("%d problem(s): human output names the On subject %d times, want %d:\n%s", count, got, count, human)
		}
		task := doc["data"].(map[string]any)["tasks"].([]any)[0].(map[string]any)
		problems, _ := task["problems"].([]any)
		if len(problems) != count || problems[0].(map[string]any)["subject"] != "job" || problems[0].(map[string]any)["message"] != "x" {
			t.Errorf("%d problem(s): run.v3 task problems = %v, want %d with subject job", count, task["problems"], count)
		}
	}
}
