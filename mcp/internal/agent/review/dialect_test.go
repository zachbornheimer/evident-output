package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

const blockSrc = `package main
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.TaskHandle) {
	task.Block("no remote")
}
`

// A removed call is API-032 at the current dialect and at a 2.0 pin, and is
// live API below 2.0, so an older pin draws no finding and keeps its pin.
func TestGoSource_TwoZeroRemovalIsAPI032FromTwoZeroOnly(t *testing.T) {
	if !hasRule(review.GoSource("main.go", blockSrc), "API-032") {
		t.Fatal("current dialect must flag Task.Block")
	}
	if !hasRule(review.GoSourceAt("main.go", blockSrc, "v2.0.0"), "API-032") {
		t.Fatal("a 2.0 pin must flag Task.Block")
	}
	old := review.GoSourceAt("main.go", blockSrc, "v1.1.0")
	if hasRule(old, "API-032") {
		t.Fatalf("a pre-2.0 desired_version must not flag Task.Block, got %+v", old.Findings)
	}
	if old.DesiredVersion != "v1.1.0" {
		t.Fatalf("DesiredVersion=%q", old.DesiredVersion)
	}
}

func hasRule(res review.Result, id string) bool {
	for _, f := range res.Findings {
		if f.RuleID == id {
			return true
		}
	}
	return false
}
