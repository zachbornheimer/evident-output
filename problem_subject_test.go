package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// warnSubjectRun is one Task that warns count times, each On("job").
func warnSubjectRun(t *testing.T, count int) (human string, doc map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "warn", Color: evo.ColorNever, Plain: true})
	task := out.Task("check jobs")
	task.Define(func(context.Context) error {
		for range count {
			task.Problem("x", evo.Severity(evo.SeverityWarning), evo.On("job"))
		}
		return nil
	})
	_ = out.Finish()
	var js bytes.Buffer
	if err := evo.WriteJSON(&js, evo.Result{Conclusion: out.Conclusion()}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	_ = out.Close()
	if err := json.Unmarshal(js.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, js.String())
	}
	return buf.String(), doc
}

// TestProblemWarning_OnSubjectReachesEveryProjection pins E-109: Warn("x",
// evo.On("job")) rendered "✓ check jobs  ! x" — the subject dropped from
// the inline row — and the run.v2 task entry carried no trace of the
// warning. Every projection now carries the warning with its subject.
func TestProblemWarning_OnSubjectReachesEveryProjection(t *testing.T) {
	for _, count := range []int{1, 2} {
		human, doc := warnSubjectRun(t, count)
		if !strings.Contains(human, "job  x") {
			t.Errorf("%d warning(s): human output drops the On subject:\n%s", count, human)
		}
		task := doc["data"].(map[string]any)["tasks"].([]any)[0].(map[string]any)
		warnings, _ := task["warnings"].([]any)
		if len(warnings) != count || warnings[0].(map[string]any)["subject"] != "job" || warnings[0].(map[string]any)["message"] != "x" {
			t.Errorf("%d warning(s): run.v2 task warnings = %v, want %d with subject job", count, task["warnings"], count)
		}
	}
}
