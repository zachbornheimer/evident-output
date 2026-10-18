package evo_test

import (
	"context"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// A run title and a Fact distinctive enough to find in any output.
const (
	configRunName  = "zq prune ~/repo"
	configFactKey  = "files-scanned"
	configFactText = "4242"
)

// scanWithFact declares one Task that records the distinctive Fact.
func scanWithFact(_ context.Context, r *evo.Run) error {
	task := r.Task("scan")
	task.Define(func(context.Context) error {
		task.Fact(configFactKey, configFactText)
		return nil
	})
	return nil
}

// TestConfigNameAndVerboseApply pins CONTRACT.md §1: Config.Name titles the
// run in every projection, and Config.Verbose shows Facts the way
// EVO_VERBOSE=1 does.
func TestConfigNameAndVerboseApply(t *testing.T) {
	t.Setenv("EVO_VERBOSE", "")
	if err := os.Unsetenv("EVO_VERBOSE"); err != nil {
		t.Fatalf("unset EVO_VERBOSE: %v", err)
	}

	for _, mode := range contractOutputs {
		t.Run("Name/"+outputName[mode], func(t *testing.T) {
			run := executeContract(context.Background(), t, evo.Config{Name: configRunName, Output: mode}, scanWithFact)
			switch mode {
			case evo.JSON:
				if got := runDocument(t, run.Stdout)["name"]; got != configRunName {
					t.Errorf("evo.run name = %v, want %q", got, configRunName)
				}
			case evo.JSONL:
				started := eventsOfType(runEvents(t, run.Stdout), "run.started")
				if len(started) != 1 || payload(started[0])["name"] != configRunName {
					t.Errorf("run.started events = %v, want one whose payload name is %q", started, configRunName)
				}
			default:
				if !strings.Contains(run.Text(), configRunName) {
					t.Errorf("text does not carry the run title %q:\n%s", configRunName, run.Text())
				}
			}
		})
	}

	for _, mode := range []evo.Output{evo.Human, evo.Plain} {
		t.Run("Verbose/"+outputName[mode], func(t *testing.T) {
			quiet := executeContract(context.Background(), t, evo.Config{Output: mode}, scanWithFact)
			if strings.Contains(quiet.Text(), configFactText) {
				t.Errorf("a Fact shows at normal verbosity:\n%s", quiet.Text())
			}
			verbose := executeContract(context.Background(), t, evo.Config{Output: mode, Verbose: true}, scanWithFact)
			for _, want := range []string{configFactKey, configFactText} {
				if !strings.Contains(verbose.Text(), want) {
					t.Errorf("Config.Verbose text does not show the Fact's %q:\n%s", want, verbose.Text())
				}
			}
		})
	}
}
