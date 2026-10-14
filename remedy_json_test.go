package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// remedyRunDocument is the slice of the run document a remedy must reach.
type remedyRunDocument struct {
	Data struct {
		Tasks []struct {
			Problems []struct {
				Remedies []remedyActionDocument `json:"remedies"`
			} `json:"problems"`
		} `json:"tasks"`
		Actions []remedyActionDocument `json:"actions"`
	} `json:"data"`
}

type remedyActionDocument struct {
	Label   string `json:"label"`
	Command *struct {
		Executable string   `json:"executable"`
		Args       []string `json:"args"`
	} `json:"command"`
}

func (a remedyActionDocument) runs(executable string, args ...string) bool {
	if a.Command == nil || a.Command.Executable != executable || len(a.Command.Args) != len(args) {
		return false
	}
	for i, arg := range args {
		if a.Command.Args[i] != arg {
			return false
		}
	}
	return true
}

func anyRuns(actions []remedyActionDocument, executable string, args ...string) bool {
	for _, a := range actions {
		if a.runs(executable, args...) {
			return true
		}
	}
	return false
}

func taskProblemRuns(doc remedyRunDocument, executable string, args ...string) bool {
	for _, task := range doc.Data.Tasks {
		for _, problem := range task.Problems {
			if anyRuns(problem.Remedies, executable, args...) {
				return true
			}
		}
	}
	return false
}

// ZYS-1182: a remedy attached with the Next/NextCommand ProblemOptions on
// Block or Fail reaches the run JSON, on the run's actions and on the
// Problem it explains.
func TestRemedy_BlockAndFailReachRunJSONActions(t *testing.T) {
	cases := map[string]func(task *Task){
		"Block": func(task *Task) { task.block("policy refused", nextCommand("tool", "--yes")) },
		"Fail":  func(task *Task) { task.fail("policy refused", nextCommand("tool", "--yes")) },
	}
	for name, resolve := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := initRun(Config{Name: "tool", Output: JSON, Facades: Facades{Stdout: &stdout, Stderr: &stderr}}, tuning{color: colorNever})
			resolve(out.Task("gate"))
			_ = out.finish()

			var doc remedyRunDocument
			if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
				t.Fatalf("stdout is not an run document: %v\n%s", err, stdout.String())
			}
			if !anyRuns(doc.Data.Actions, "tool", "--yes") {
				t.Fatalf("data.actions = %+v, want the remedy command tool --yes", doc.Data.Actions)
			}
			if !taskProblemRuns(doc, "tool", "--yes") {
				t.Fatalf("data.tasks = %+v, want the remedy on the Problem it explains", doc.Data.Tasks)
			}
		})
	}
}

// Confirm's policy block rides the same path (ZYS-1182 removed its hidden
// task-level attachment).
func TestRemedy_ConfirmPolicyHintReachesRunJSONActions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := initRun(Config{Name: "tool", Output: JSON, Facades: Facades{Stdout: &stdout, Stderr: &stderr}}, tuning{color: colorNever})
	out.Confirm("delete?")
	_ = out.finish()

	var doc remedyRunDocument
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not an run document: %v\n%s", err, stdout.String())
	}
	if len(doc.Data.Tasks) != 1 || len(doc.Data.Tasks[0].Problems) != 1 {
		t.Fatalf("data.tasks = %+v, want one task with one policy-block Problem", doc.Data.Tasks)
	}
	remedies := doc.Data.Tasks[0].Problems[0].Remedies
	if len(remedies) != 1 || !strings.Contains(remedies[0].Label, "--yes") {
		t.Fatalf("remedies = %+v, want the --yes hint on the policy-block Problem", remedies)
	}
}

// ZYS-1182 decision: the canonical way for a Define callback to fail with a
// remedy is Fail(summary, Detail(err), remedy) followed by `return nil`. It
// records exactly one Problem (summary, detail, remedy), fails the Task, and
// exits 2.
func TestRemedy_FailInsideDefineCarriesDetailAndRemedyToRunJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := initRun(Config{Name: "zq", Output: JSON, Facades: Facades{Stdout: &stdout, Stderr: &stderr}}, tuning{color: colorNever})
	lint := out.Task("lint")
	lint.Define(func(context.Context) error {
		lint.fail("lint failed", Detail("exit status 1"), nextCommand("zq", "fix"))
		return nil
	})
	if err := lint.wait(); err == nil {
		t.Fatal("Wait() = nil, want an error for a failed Task")
	}
	_ = out.finish()

	var doc struct {
		ExitCode int `json:"exit_code"`
		Data     struct {
			Tasks []struct {
				State    string `json:"state"`
				Problems []struct {
					Message  string                 `json:"message"`
					Detail   string                 `json:"detail"`
					Remedies []remedyActionDocument `json:"remedies"`
				} `json:"problems"`
			} `json:"tasks"`
			Actions []remedyActionDocument `json:"actions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not an run document: %v\n%s", err, stdout.String())
	}
	if doc.ExitCode != 2 {
		t.Fatalf("exit_code = %d, want 2", doc.ExitCode)
	}
	if len(doc.Data.Tasks) != 1 || doc.Data.Tasks[0].State != "failed" || len(doc.Data.Tasks[0].Problems) != 1 {
		t.Fatalf("data.tasks = %+v, want one failed task with exactly one Problem", doc.Data.Tasks)
	}
	problem := doc.Data.Tasks[0].Problems[0]
	if problem.Message != "lint failed" || problem.Detail != "exit status 1" || !anyRuns(problem.Remedies, "zq", "fix") {
		t.Fatalf("problem = %+v, want summary, detail, and the zq fix remedy", problem)
	}
	if !anyRuns(doc.Data.Actions, "zq", "fix") {
		t.Fatalf("data.actions = %+v, want the zq fix remedy", doc.Data.Actions)
	}
}
