package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

func TestFP004_PlaceholderDoingText(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.Task) {
  task.Doing("working")
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "FP-004" {
			found = true
			if f.Suggestion == "" {
				t.Error("FP-004 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected FP-004 on placeholder Doing text: %+v", res.Findings)
	}
}

func TestFP004_NoFalsePositiveOnDomainObjectDoing(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.Task) {
  task.Doing("scanning ~/Developer/Personal/zq")
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-004" {
			t.Fatalf("false positive FP-004 on domain-object Doing text: %+v", res.Findings)
		}
	}
}

func TestFP001_HeavyIOBeforeEvoMain(t *testing.T) {
	bad := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  data, _ := os.ReadFile("config.toml")
  _ = data
  os.Exit(evo.Main(evo.Config{Name: "t"}, run))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "FP-001" {
			found = true
			if f.Line == 0 {
				t.Error("FP-001 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected FP-001 on I/O before evo.Main: %+v", res.Findings)
	}
}

func TestFP001_NoFalsePositiveWhenMainFirst(t *testing.T) {
	good := `package main
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  os.Exit(evo.Main(evo.Config{Name: "t"}, run))
}
func run(ctx context.Context, r *evo.Run) error {
  scan := r.Task("scan")
  scan.Doing("reading config")
  scan.Define(func(ctx context.Context) error {
    _, err := os.ReadFile("config.toml")
    return err
  })
  return nil
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-001" || f.RuleID == "FP-002" {
			t.Fatalf("false positive %s when Main runs first: %+v", f.RuleID, res.Findings)
		}
	}
}

func TestFP002_InventoryBeforeFirstTask(t *testing.T) {
	bad := `package app
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run) error {
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  r.Task("inventory").Define(func(ctx context.Context) error { return err })
  return nil
}
`
	res := review.GoSource("purge.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID != "FP-002" {
			continue
		}
		found = true
		if f.Line == 0 {
			t.Error("FP-002 missing line")
		}
		for _, want := range []string{"Task", "Group", "Sequence", "Inventory"} {
			if !strings.Contains(f.Suggestion, want) {
				t.Fatalf("FP-002 suggestion must name Task/Group/Sequence before Inventory, got %q", f.Suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("expected FP-002 on Inventory before the first Task (zq purge canary): %+v", res.Findings)
	}
}

func TestFP002_NoFalsePositiveWhenTaskBeforeInventory(t *testing.T) {
	good := `package app
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run) error {
  inv := r.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func(ctx context.Context) error {
    _, err := purge.Inventory(ctx, opt)
    return err
  })
  return nil
}
`
	res := review.GoSource("purge.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-002" {
			t.Fatalf("false positive FP-002 when Task is declared before Inventory: %+v", res.Findings)
		}
	}
}

func TestFP002_HeavyIOBeforeFirstEntity(t *testing.T) {
	bad := `package main
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  data, _ := os.ReadFile("config.toml")
  r.Task("scan").Doing(string(data))
  return nil
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "FP-002" {
			found = true
			if f.Line == 0 {
				t.Error("FP-002 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected FP-002 on I/O before the first entity: %+v", res.Findings)
	}
}

func TestFP003_DoingSetOnceBeforeSilentSubprocess(t *testing.T) {
	bad := `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(t *evo.Task, cmd *exec.Cmd) {
  t.Doing("uploading")
  cmd.Run()
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "FP-003" {
			found = true
			if f.Line == 0 {
				t.Error("FP-003 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected FP-003 on stale Doing before subprocess: %+v", res.Findings)
	}
}

func TestFP003_NoFalsePositiveWithTaskWriter(t *testing.T) {
	good := `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(t *evo.Task, cmd *exec.Cmd) {
  t.Doing("uploading")
  cmd.Stdout = t.Writer()
  cmd.Run()
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-003" {
			t.Fatalf("false positive FP-003 when Task.Writer wires the child: %+v", res.Findings)
		}
	}
}

// fp003Program wraps two Task callbacks the way run bodies declare them: a
// progress loop in one, a silent child in the other.
const fp003Program = `package main

import (
	"context"
	"fmt"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) error {
	idx := r.Task("index")
	idx.Define(func(ctx context.Context) error {
		for i := range 3 {
			idx.Progress(i, 3).Doing(fmt.Sprintf("chunk %d", i))
		}
		return nil
	})
	r.Task("upload").After(idx).Define(func(ctx context.Context) error {
CALLBACK_BODY
	})
	return nil
}
`

func fp003Findings(t *testing.T, callbackBody string) []review.Finding {
	t.Helper()
	res := review.GoSource("main.go", strings.Replace(fp003Program, "CALLBACK_BODY", callbackBody, 1))
	var found []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "FP-003" {
			found = append(found, f)
		}
	}
	return found
}

// TestFP003IgnoresExecInAnotherTasksCallback pins that FP-003 judges one
// callback body at a time: a Doing in one Task's callback is not stale
// because a different Task's callback starts a child.
func TestFP003IgnoresExecInAnotherTasksCallback(t *testing.T) {
	if found := fp003Findings(t, `		return exec.CommandContext(ctx, "true").Run()`); len(found) != 0 {
		t.Fatalf("FP-003 paired a Doing with a child in another callback: %+v", found)
	}
	if found := fp003Findings(t, `		return nil`); len(found) != 0 {
		t.Fatalf("FP-003 fired with no child anywhere: %+v", found)
	}
}

// TestFP003StillFiresInsideOneCallback pins the other half: the stale Doing
// and the silent child in the same callback are reported at the Doing, and a
// Writer wired in a different callback does not excuse them.
func TestFP003StillFiresInsideOneCallback(t *testing.T) {
	found := fp003Findings(t, `		idx.Doing("uploading")
		return exec.CommandContext(ctx, "true").Run()`)
	if len(found) != 1 || found[0].Line != 20 {
		t.Fatalf("FP-003 findings = %+v, want one at line 20 (the Doing)", found)
	}
}

func TestTAX001_HandAssembledSkipCount(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, n int) {
  r.Summary(fmt.Sprintf("%d skipped (dirty/unpushed/main)", n))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TAX-001" {
			found = true
			if f.Line == 0 {
				t.Error("TAX-001 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected TAX-001 on hand-assembled skip count: %+v", res.Findings)
	}
}

func TestTAX001_NoFalsePositiveOnStructuredReason(t *testing.T) {
	good := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  r.Task("main").Define(func(context.Context) error { return evo.Exclude("protected") })
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "TAX-001" {
			t.Fatalf("false positive TAX-001 on structured reason: %+v", res.Findings)
		}
	}
}

func TestPROG001_DoingStringSmugglesProgress(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(task *evo.Task, done, total int) {
  task.Doing("scanning %d/%d", done, total)
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "PROG-001" {
			found = true
			if f.Line == 0 {
				t.Error("PROG-001 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected PROG-001 on Doing string smuggling %%d/%%d: %+v", res.Findings)
	}
}

func TestPROG001_NoFalsePositiveOnPlainDoing(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.Task) {
  task.Doing("scanning")
  task.Progress(4, 10)
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "PROG-001" {
			t.Fatalf("false positive PROG-001 on plain Doing + absolute Progress: %+v", res.Findings)
		}
	}
}
