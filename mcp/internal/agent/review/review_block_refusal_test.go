package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// blockRefusalRules are the rules that used to rewrite a Block site into a
// failure. In 1.1 Block is a statement: Block then return nil inside Define
// keeps the Task Blocked (a refusal with no error). With a real error, Block
// then return err also renders Blocked and Wait() returns the error.
var blockRefusalRules = map[string]bool{"API-034": true, "API-036": true, "API-040": true}

func blockRefusalSrc(body string) string {
	return `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, name string, err error) {
  task.Define(func(ctx context.Context) error {
    ` + body + `
  })
  _ = fmt.Sprint(name, err)
}
`
}

// TestBlockRefusal_ReviewLoopKeepsTheRefusal pins E-105 for 1.1: Block then
// return nil inside Define is the refusal. Review must not rewrite it to
// Failf/Blockf or fmt.Errorf.
func TestBlockRefusal_ReviewLoopKeepsTheRefusal(t *testing.T) {
	cases := []struct{ name, body string }{
		{"Block then return nil", "task.Block(\"refused x\")\n    return nil"},
		{"Block(Sprintf) then return nil", "task.Block(fmt.Sprintf(\"refused %s\", name))\n    return nil"},
		{"Block with Detail then return nil", "task.Block(\"refused x\", evo.Detail(err.Error()))\n    return nil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, f := range review.GoSource("block.go", blockRefusalSrc(tc.body)).Findings {
				if blockRefusalRules[f.RuleID] {
					t.Fatalf("%s flagged the 1.1 Block refusal: %+v", f.RuleID, f)
				}
				if strings.Contains(f.Suggestion, "Blockf") || strings.Contains(f.Suggestion, "Failf") {
					t.Fatalf("suggestion teaches a removed printf verb: %q", f.Suggestion)
				}
			}
		})
	}
}
