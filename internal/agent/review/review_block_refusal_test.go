package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// blockRefusalRules are the rules that flag a Block site whose refusal is
// discarded by a bare `return nil` right after it. E-105: the fix is never
// a bare `return fmt.Errorf(...)` (which would conclude the Task Failed,
// exit 2, instead of Blocked): it keeps the original Block call and returns
// the wrapped error right after it instead of nil.
var blockRefusalRules = map[string]bool{"API-034": true}

// blockRefusalLoop is one Block site inside Define, and the source after
// the review's suggested rewrite.
type blockRefusalLoop struct {
	name, before, after string
}

var blockRefusalLoops = []blockRefusalLoop{
	{
		name:   "Block then return nil",
		before: "task.Block(\"refused x\")\n    return nil",
		after:  "task.Block(\"refused x\")\n    return wrapped",
	},
	{
		name:   "Block(Sprintf) then return nil",
		before: "task.Block(fmt.Sprintf(\"refused %s\", name))\n    return nil",
		after:  "task.Block(fmt.Sprintf(\"refused %s\", name))\n    return wrapped",
	},
}

// TestBlockThenReturnErr_NoFalsePositive pins that `task.Block(...)`
// immediately followed by `return err` (or any other wrapped error) is NOT
// flagged: since Blockf was removed in 1.1, Block is statement-form and
// terminalizes the Task immediately, so returning the same (or a folded,
// wrapped) error right after it to propagate up the Go call stack is the
// sanctioned idiom, not a double-resolve — the scheduler's own
// re-resolution of an already-terminal task is a harmless no-op.
func TestBlockThenReturnErr_NoFalsePositive(t *testing.T) {
	src := blockRefusalSrc("task.Block(\"refused x\")\n    return err")
	for _, f := range review.GoSource("block.go", src).Findings {
		if blockRefusalRules[f.RuleID] || f.RuleID == "API-040" {
			t.Fatalf("false positive on Block then return err (the sanctioned idiom since 1.1): %+v", f)
		}
	}
}

func blockRefusalSrc(body string) string {
	return `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, name string, err, wrapped error) {
  task.Define(func(ctx context.Context) error {
    ` + body + `
  })
  _ = fmt.Sprint(name, err, wrapped)
}
`
}

// TestBlockRefusal_ReviewLoopKeepsTheRefusal pins E-105: every Block form
// inside Define gets exactly one suggestion, that suggestion keeps the
// Block call and returns the wrapped error separately (never a bare
// fmt.Errorf that would conclude the Task Failed instead), and the
// rewritten source is clean for both rules — so the MUST-loop ends with
// the Task still concluding Blocked.
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
			s := hits[0].Suggestion
			if !strings.Contains(s, "task.Block(") || !strings.Contains(s, "return wrapped") {
				t.Fatalf("%s suggestion = %q, want it to keep task.Block(...) and return wrapped", hits[0].RuleID, s)
			}
			if strings.Contains(s, "Blockf") {
				t.Fatalf("%s suggestion = %q must not reference the removed Blockf", hits[0].RuleID, s)
			}
			for _, f := range review.GoSource("block.go", blockRefusalSrc(tc.after)).Findings {
				if blockRefusalRules[f.RuleID] {
					t.Fatalf("the applied suggestion is flagged again: %+v", f)
				}
			}
		})
	}
}
