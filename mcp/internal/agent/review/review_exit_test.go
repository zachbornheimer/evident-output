package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// TestExitBypass_OneFixThatCompiles pins API-018/EVO-EXIT-001: the one
// os.Exit defect reports under both IDs with one shared suggestion, and
// neither steers to a spelling that does not compile (Run takes ctx;
// Conclusion is a field, not a method).
func TestExitBypass_OneFixThatCompiles(t *testing.T) {
	src := `package main
import (
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
func main() {
	evo.Init(evo.Config{Title: "t"})
	os.Exit(3)
}
`
	res := review.GoSource("main.go", src)
	suggestions := map[string]string{}
	for _, f := range res.Findings {
		if f.RuleID != "API-018" && f.RuleID != "EVO-EXIT-001" {
			continue
		}
		suggestions[f.RuleID] = f.Suggestion
		for _, text := range []string{f.Message, f.Suggestion} {
			if strings.Contains(text, "Conclusion().") || strings.Contains(text, "evo.Run(run)") {
				t.Errorf("%s steers to a spelling that does not compile: %q", f.RuleID, text)
			}
		}
	}
	if len(suggestions) != 2 {
		t.Fatalf("want API-018 and EVO-EXIT-001, got %v", suggestions)
	}
	if suggestions["API-018"] != suggestions["EVO-EXIT-001"] {
		t.Fatalf("one defect, two fixes:\nAPI-018:      %s\nEVO-EXIT-001: %s", suggestions["API-018"], suggestions["EVO-EXIT-001"])
	}
}
