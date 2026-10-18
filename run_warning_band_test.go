package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestRunWarningMarksTheConclusionWarned: a run that could not save its
// manifest says so on its own line and ends with the Warned modifier on its
// band, while the outcome stays what its Tasks did.
func TestRunWarningMarksTheConclusionWarned(t *testing.T) {
	run := manifestNotSavedScenario(t, evo.Plain)
	text := run.Text()
	for _, want := range []string{"manifest not saved", "[changed · warned]"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}
