package facts_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func factTask(out *evo.Output, name string, fail bool) *evo.TaskHandle {
	task := out.Task(name)
	task.Define(func(context.Context) error {
		task.Fact("path", "/secret/place")
		if fail {
			return errors.New("boom")
		}
		return nil
	})
	_ = task.Wait()
	return task
}

func TestC11_001_FactsAreHiddenInNormalOutputOnEveryOutcome(t *testing.T) {
	for name, fail := range map[string]bool{"success": false, "failure": true} {
		out, buf := harness.New(t)
		factTask(out, "job", fail)
		if text := harness.Text(out, buf); strings.Contains(text, "/secret/place") {
			t.Errorf("%s: normal output shows an ordinary Fact:\n%s", name, text)
		}
	}
}

func TestC11_002_VerboseOutputShowsFacts(t *testing.T) {
	out, buf := harness.New(t, func(c *evo.Config) { c.Verbosity = evo.VerbosityVerbose })
	factTask(out, "job", false)
	if text := harness.Text(out, buf); !strings.Contains(text, "path  /secret/place") {
		t.Fatalf("verbose output lacks the Fact:\n%s", text)
	}
}

func TestC11_003_MachineOutputAlwaysCarriesFacts(t *testing.T) {
	out, _ := harness.New(t)
	factTask(out, "job", false)
	doc := harness.RunDocument(t, out)
	tasks := harness.Objects(t, harness.Object(t, doc, "data"), "tasks")
	facts := harness.Objects(t, tasks[0], "facts")
	if len(facts) != 1 || facts[0]["name"] != "path" || facts[0]["value"] != "/secret/place" {
		t.Fatalf("facts = %v", facts)
	}
}

func TestC11_004_ConfigFactsRenderWithTheHeader(t *testing.T) {
	out, buf := harness.New(t, func(c *evo.Config) {
		c.Facts = []evo.FactRecord{{Name: "root", Value: "/repo"}}
	})
	if err := harness.Succeed(out.Task("job")); err != nil {
		t.Fatal(err)
	}
	text := harness.Text(out, buf)
	fact, task := strings.Index(text, "root  /repo"), strings.Index(text, "job")
	if fact < 0 || task < 0 || fact > task {
		t.Fatalf("Config.Facts not rendered ahead of the tasks:\n%s", text)
	}
}

func TestC11_005_FailingTaskDoesNotPromoteUnrelatedFacts(t *testing.T) {
	out, buf := harness.New(t)
	factTask(out, "job", true)
	text := harness.Text(out, buf)
	if !strings.Contains(text, "boom") || strings.Contains(text, "/secret/place") {
		t.Fatalf("failure output:\n%s", text)
	}
}
