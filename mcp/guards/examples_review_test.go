package guards_test

import (
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// TestExamplesPassReview: agents copy examples verbatim, so the shipped
// examples must pass the same review the MCP autofixer runs on consumer
// code. Any finding here is an anti-pattern the release would teach.
func TestExamplesPassReview(t *testing.T) {
	dir := filepath.Join(moduleRoot(t), "examples")
	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatalf("review %s: %v", dir, err)
	}
	for _, f := range res.Findings {
		t.Errorf("%s:%d %s %s: %s", f.File, f.Line, f.RuleID, f.Severity, f.Message)
	}
	if res.RecheckRequired {
		t.Error("review of examples/ reports recheck_required=true")
	}
}
