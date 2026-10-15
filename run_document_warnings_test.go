package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestRunDocumentCarriesRunWarnings pins CONTRACT.md section 4: a warning about
// the run itself (a manifest it could not save) is in `data.problems` of
// `evo.run`, the same warning Human and Plain print on their own line and
// `· warned` band, so no projection hides state another shows.
func TestRunDocumentCarriesRunWarnings(t *testing.T) {
	run := manifestNotSavedScenario(t, evo.JSON)
	data, _ := runDocument(t, run.Stdout)["data"].(map[string]any)
	problems, _ := data["problems"].([]any)
	if len(problems) != 1 {
		t.Fatalf("data.problems = %v, want the one run warning", data["problems"])
	}
	problem, _ := problems[0].(map[string]any)
	if summary, _ := problem["message"].(string); !strings.Contains(summary, "manifest not saved") {
		t.Errorf("run warning = %v, want its message to say the manifest was not saved", problem)
	}
}
