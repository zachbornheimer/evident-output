package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// TestBlockedAsError_LaterOutsideDefineBlockStillFlagged pins the DOM-011
// file-wide-suppression bug: the detector used to compute the file's FIRST
// Block/BlockedBy line once and, if that first Block happened to sit
// inside a Define callback, skip the whole file — so a second, run-level
// `Block; return err` later in the same file was silently never checked.
// Every Block line must be judged on its own Define membership.
func TestBlockedAsError_LaterOutsideDefineBlockStillFlagged(t *testing.T) {
	src := `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output, task *evo.TaskHandle, err error) error {
  task.Define(func(ctx context.Context) error {
    if err != nil {
      task.Block("first, inside Define")
      return err
    }
    return nil
  })
  if err != nil {
    task.Block("second, run-level")
    return err
  }
  _ = fmt.Sprint(out)
  return nil
}
`
	res := review.GoSource("blocked.go", src)
	var hits []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			hits = append(hits, f)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want exactly one DOM-011 finding (the run-level Block, not the Define-owned one), got %d: %+v", len(hits), res.Findings)
	}
}

// TestBlockedAsError_HelperReachableFromDefine pins the second half of the
// AST-owner switch: a same-file helper function reached only through a
// Define callback is still inside Define (defineReachableBlocks), which a
// text brace-counter cannot see across function boundaries. A `Block;
// return err` inside such a helper must not fire DOM-011.
func TestBlockedAsError_HelperReachableFromDefine(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func refuse(task *evo.TaskHandle, err error) error {
  task.Block("refused via helper")
  return err
}
func run(task *evo.TaskHandle, err error) error {
  task.Define(func(ctx context.Context) error {
    return refuse(task, err)
  })
  return nil
}
`
	res := review.GoSource("blocked.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("Block+return err inside a Define-reachable helper flagged DOM-011: %+v", f)
		}
	}
}
