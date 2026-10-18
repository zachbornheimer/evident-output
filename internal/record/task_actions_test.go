package record

import (
	"testing"
	"time"
)

func TestTaskAppendActionKeepsEveryNextStepInOrder(t *testing.T) {
	task := NewRun().NewTask(TaskInit{})
	task.AppendAction(Action{Label: "first"})
	task.AppendAction(Action{Label: "second"})

	got := task.Actions()
	if len(got) != 2 || got[0].Label != "first" || got[1].Label != "second" {
		t.Fatalf("Actions() = %+v, want first then second", got)
	}
	got[0].Label = "mutated"
	if task.Actions()[0].Label != "first" {
		t.Error("Actions() returned the Task's own slice, not a copy")
	}
	if len(task.Truth().Actions) != 2 {
		t.Errorf("Truth().Actions = %+v, want both next steps", task.Truth().Actions)
	}
}

func TestTaskMarkActivityRecordsTheLatestMoment(t *testing.T) {
	task := NewRun().NewTask(TaskInit{})
	if !task.ActivityAt().IsZero() {
		t.Fatalf("ActivityAt() = %v before any report, want zero", task.ActivityAt())
	}
	first := time.Unix(100, 0)
	later := time.Unix(200, 0)
	task.MarkActivity(first)
	task.MarkActivity(later)
	if !task.ActivityAt().Equal(later) || !task.Truth().ActivityAt.Equal(later) {
		t.Errorf("ActivityAt() = %v, want %v", task.ActivityAt(), later)
	}
}

func TestContainerSetSummarySanitizesAndClears(t *testing.T) {
	container := NewRun().NewContainer()
	container.SetSummary("done\x1b[31m red")
	if got := container.Summary(); got != SanitizeText("done\x1b[31m red") || got == "done\x1b[31m red" {
		t.Errorf("Summary() = %q, want the sanitized text", got)
	}
	container.SetSummary("")
	if got := container.Summary(); got != "" {
		t.Errorf("Summary() = %q after clearing, want empty", got)
	}
}
