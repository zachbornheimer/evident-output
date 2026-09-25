package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// blockRefusalRules are the rules that rewrite a Fail/Block site. E-105:
// each of them turned a Define's refusal into a failure — they suggested
// `return fmt.Errorf(...)` for a Block site (a plain error concludes
// Failed, exit 2) or flagged `return task.Blockf(...)`, the one spelling
// that refuses inside Define.
var blockRefusalRules = map[string]bool{"API-034": true, "API-036": true, "API-040": true}

// blockRefusalLoop is one Block site inside Define, the line review should
// rewrite it to, and the source after that rewrite.
type blockRefusalLoop struct {
	name, before, want string
}

var blockRefusalLoops = []blockRefusalLoop{
	{
		name:   "Block then return nil",
		before: "task.Block(\"refused x\")\n    return nil",
		want:   `return task.Blockf("refused x")`,
	},
	{
		name:   "Block(Sprintf) then return nil",
		before: "task.Block(fmt.Sprintf(\"refused %s\", name))\n    return nil",
		want:   `return task.Blockf("refused %s", name)`,
	},
	{
		name:   "Block then return err",
		before: "task.Block(\"refused x\")\n    return err",
		want:   `return task.Blockf("<context>: %w", err)`,
	},
}

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

// TestBlockRefusal_ReviewLoopKeepsTheRefusal pins E-105: every Block form
// inside Define gets exactly one suggestion, that suggestion is
// `return task.Blockf(...)` (never fmt.Errorf), and the rewritten source is
// clean for all three rules — so the MUST-loop ends with the Task still
// concluding Blocked.
func TestBlockRefusal_ReviewLoopKeepsTheRefusal(t *testing.T) {
	for _, tc := range blockRefusalLoops {
		t.Run(tc.name, func(t *testing.T) {
			var hits []review.Finding
			for _, f := range review.GoSource("block.go", blockRefusalSrc(tc.before)).Findings {
				if blockRefusalRules[f.RuleID] {
					hits = append(hits, f)
				}
			}
			if len(hits) != 1 {
				t.Fatalf("want one Fail/Block rewrite, got %d: %+v", len(hits), hits)
			}
			if s := hits[0].Suggestion; !strings.Contains(s, "`"+tc.want+"`") || strings.Contains(s, "fmt.Errorf") {
				t.Fatalf("%s suggestion = %q, want %q and no fmt.Errorf", hits[0].RuleID, s, tc.want)
			}
			for _, f := range review.GoSource("block.go", blockRefusalSrc(tc.want)).Findings {
				if blockRefusalRules[f.RuleID] {
					t.Fatalf("the applied suggestion is flagged again: %+v", f)
				}
			}
		})
	}
}
