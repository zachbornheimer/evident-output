package evo_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The options every ending below carries, and what each must record.
const (
	optionDetail  = "3 issues in main.go"
	optionCode    = "lint"
	optionSubject = "origin/main"
	optionLabel   = "run gofmt -w ."
)

func endingOptions() []evo.ProblemOption {
	return []evo.ProblemOption{
		evo.Detail(optionDetail),
		evo.Code(optionCode),
		evo.On(optionSubject),
		evo.Next(evo.Action{Label: optionLabel}),
	}
}

// endingWithOptions builds one ending through its exported func. Exclude is
// called by reflection: CONTRACT.md §4 gives it ProblemOptions, and a test
// that called it with them directly would not compile until it took them.
func endingWithOptions(t *testing.T, ending string, summary string) error {
	t.Helper()
	opts := endingOptions()
	switch ending {
	case "Fail":
		return evo.Fail(summary, opts...)
	case "Refuse":
		return evo.Refuse(summary, opts...)
	}
	fn := reflect.ValueOf(evo.Exclude)
	if !fn.Type().IsVariadic() || fn.Type().In(fn.Type().NumIn()-1).Elem() != reflect.TypeFor[evo.ProblemOption]() {
		t.Fatalf("Exclude has type %s; CONTRACT.md §4 gives it opts ...ProblemOption", fn.Type())
	}
	args := []reflect.Value{reflect.ValueOf(summary)}
	for _, opt := range opts {
		args = append(args, reflect.ValueOf(opt))
	}
	err, _ := fn.Call(args)[0].Interface().(error)
	return err
}

// TestEndingCarriesProblemOptions pins CONTRACT.md §4: Fail, Refuse and
// Exclude record every ProblemOption on the Problem their ending creates.
func TestEndingCarriesProblemOptions(t *testing.T) {
	for _, ending := range []string{"Fail", "Refuse", "Exclude"} {
		for _, mode := range []evo.Output{evo.JSON, evo.Human, evo.Plain} {
			t.Run(ending+"/"+outputName[mode], func(t *testing.T) {
				summary := strings.ToLower(ending) + " summary"
				err := endingWithOptions(t, ending, summary)
				run := singleTask(nil, func(context.Context) error { return err })(t, mode)
				if mode == evo.JSON {
					assertTaskProblemCarriesOptions(t, runDocument(t, run.Stdout), summary)
					return
				}
				if ending == "Exclude" {
					return // an Excluded row folds into a tally; JSON keeps every item
				}
				for _, want := range []string{summary, optionDetail, optionLabel} {
					if !strings.Contains(run.Text(), want) {
						t.Errorf("text does not show %q:\n%s", want, run.Text())
					}
				}
			})
		}
	}
}

// assertTaskProblemCarriesOptions finds the Problem named summary on any Task
// and checks it recorded each option.
func assertTaskProblemCarriesOptions(t *testing.T, doc map[string]any, summary string) {
	t.Helper()
	var problem map[string]any
	for _, task := range documentTasks(t, doc) {
		list, _ := task["problems"].([]any)
		for _, item := range list {
			if p, _ := item.(map[string]any); p != nil && p["message"] == summary {
				problem = p
			}
		}
	}
	if problem == nil {
		t.Fatalf("no task problem with message %q in evo.run data.tasks", summary)
	}
	for key, want := range map[string]string{"code": optionCode, "detail": optionDetail, "subject": optionSubject} {
		if got := problem[key]; got != want {
			t.Errorf("problem %s = %v, want %q", key, got, want)
		}
	}
	remedies, _ := problem["remedies"].([]any)
	for _, item := range remedies {
		if r, _ := item.(map[string]any); r != nil && r["label"] == optionLabel {
			return
		}
	}
	t.Errorf("problem remedies = %v, want one labelled %q", problem["remedies"], optionLabel)
}
