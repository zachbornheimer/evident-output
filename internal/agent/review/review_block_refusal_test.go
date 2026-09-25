package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// blockRefusalRules are the rules that touch a Fail/Block site inside
// Define. E-105: turning a Block's refusal into a returned error concludes
// the Task Failed (exit 2), not Blocked — Failf/Blockf's removal in 1.1
// left no way to resolve Blocked in a single return, so the paved path is
// to keep the Block statement (the only way to reach Blocked) and change
// only what follows it: a discarded `return nil` becomes `return err`.
var blockRefusalRules = map[string]bool{"API-034": true, "API-040": true}

// blockRefusalLoop is one Block site inside Define, the line review should
// rewrite the trailing return to, and the source after that rewrite.
type blockRefusalLoop struct {
	name, before, want string
}

var blockRefusalLoops = []blockRefusalLoop{
	{
		name:   "Block then return nil",
		before: "task.Block(\"refused x\")\n    return nil",
		want:   "task.Block(\"refused x\")\n    return err",
	},
	{
		name:   "Block(Sprintf) then return nil",
		before: "task.Block(fmt.Sprintf(\"refused %s\", name))\n    return nil",
		want:   "task.Block(fmt.Sprintf(\"refused %s\", name))\n    return err",
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

// TestBlockRefusal_ReviewLoopKeepsTheRefusal pins E-105 as it stands after
// Failf/Blockf's removal: every Block-then-return-nil form inside Define
// gets exactly one suggestion, that suggestion keeps the Block statement
// (never rewrites it to a bare fmt.Errorf, which would conclude the Task
// Failed instead) and proposes `return err` in place of the discarded nil,
// and the rewritten source is clean — so the MUST-loop ends with the Task
// still concluding Blocked.
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
				t.Fatalf("want one Fail/Block finding, got %d: %+v", len(hits), hits)
			}
			if s := hits[0].Suggestion; !strings.Contains(s, "return err") || strings.Contains(s, "fmt.Errorf(") {
				t.Fatalf("%s suggestion = %q, want it to keep the Block statement and propose return err, never fmt.Errorf", hits[0].RuleID, s)
			}
			for _, f := range review.GoSource("block.go", blockRefusalSrc(tc.want)).Findings {
				if blockRefusalRules[f.RuleID] {
					t.Fatalf("the applied suggestion is flagged again: %+v", f)
				}
			}
		})
	}
}

// TestBlockThenReturnErr_NotFlagged pins the shape blockRefusalLoops rewrite
// to: a Block statement immediately followed by `return err` (or any
// non-nil error) is not a double-resolve and not ceremony to remove — Block
// is the only way to conclude the Task Blocked, so this is already the
// correct, final form and must not be flagged by any rule in this family.
func TestBlockThenReturnErr_NotFlagged(t *testing.T) {
	src := blockRefusalSrc("task.Block(\"refused x\")\n    return err")
	for _, f := range review.GoSource("block.go", src).Findings {
		if blockRefusalRules[f.RuleID] {
			t.Fatalf("Block then return err flagged as %s, want it left alone: %+v", f.RuleID, f)
		}
	}
}
