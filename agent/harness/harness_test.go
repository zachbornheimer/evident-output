package harness_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/harness"
)

func TestAgentHarness_DefaultScenariosPass(t *testing.T) {
	results := harness.Run()
	if len(results) == 0 {
		t.Fatal("no scenarios")
	}
	for _, r := range results {
		if !r.Passed {
			t.Errorf("scenario %s failed: %s missing=%v", r.ID, r.Detail, r.MissingRules)
		}
	}
}
