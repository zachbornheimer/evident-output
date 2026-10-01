package evo_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// remedyRunDocument is the slice of the evo.run document a remedy must reach.
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
// Block or Fail reaches the evo.run JSON, on the run's actions and on the
// Problem it explains.
func TestRemedy_BlockAndFailReachRunJSONActions(t *testing.T) {
	cases := map[string]func(task *evo.TaskHandle){
		"Block": func(task *evo.TaskHandle) { task.Block("policy refused", evo.NextCommand("tool", "--yes")) },
		"Fail":  func(task *evo.TaskHandle) { task.Fail("policy refused", evo.NextCommand("tool", "--yes")) },
	}
	for name, resolve := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := evo.Init(evo.Config{Isolated: true, Title: "tool", Stdout: &stdout, Stderr: &stderr, Plain: true, Color: evo.ColorNever, Format: evo.FormatJSON})
			resolve(out.Task("gate"))
			_ = out.Finish()

			var doc remedyRunDocument
			if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
				t.Fatalf("stdout is not an evo.run document: %v\n%s", err, stdout.String())
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
	out := evo.Init(evo.Config{Isolated: true, Title: "tool", Stdout: &stdout, Stderr: &stderr, Plain: true, Color: evo.ColorNever, Format: evo.FormatJSON})
	out.Confirm("delete?")
	_ = out.Finish()

	var doc remedyRunDocument
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not an evo.run document: %v\n%s", err, stdout.String())
	}
	if len(doc.Data.Tasks) != 1 || len(doc.Data.Tasks[0].Problems) != 1 {
		t.Fatalf("data.tasks = %+v, want one task with one policy-block Problem", doc.Data.Tasks)
	}
	remedies := doc.Data.Tasks[0].Problems[0].Remedies
	if len(remedies) != 1 || !strings.Contains(remedies[0].Label, "--yes") {
		t.Fatalf("remedies = %+v, want the --yes hint on the policy-block Problem", remedies)
	}
}
