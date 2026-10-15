package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

func TestTERM015_TTYPassthroughWithoutSuspend(t *testing.T) {
	bad := `package p
import (
  "os"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(r *evo.Run) error {
  cmd := exec.Command("zq", "setup")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr
  return cmd.Run()
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TERM-015" {
			found = true
			if f.Line == 0 {
				t.Error("TERM-015 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected TERM-015 on tty passthrough: %+v", res.Findings)
	}
}

func TestTERM015_NoFalsePositiveWithTaskWriter(t *testing.T) {
	good := `package p
import (
  "context"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(r *evo.Run) error {
  task := r.Task("build")
  task.Define(func(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "go", "build", "./...")
    cmd.Stdout = task.Writer()
    cmd.Stderr = task.Writer()
    return cmd.Run()
  })
  return nil
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "TERM-015" {
			t.Fatalf("false positive TERM-015 when Task.Writer captures the child: %+v", res.Findings)
		}
	}
}

func TestTERM015_NoFalsePositiveInsideSuspend(t *testing.T) {
	good := `package p
import (
  "os"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(r *evo.Run) error {
  return r.Suspend(func() error {
    cmd := exec.Command("zq", "login")
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
  })
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "TERM-015" {
			t.Fatalf("false positive TERM-015 for a child that owns the terminal inside Suspend: %+v", res.Findings)
		}
	}
}

func TestCONFIRM001_HandRolledConfirmDetected(t *testing.T) {
	bad := `package p
import (
  "bufio"
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(r *evo.Run) error {
  reader := bufio.NewReader(os.Stdin)
  fmt.Print("delete origin/production-hotfix? [y/N] ")
  _, _ = reader.ReadString('\n')
  return nil
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "CONFIRM-001" {
			found = true
			if f.Line == 0 {
				t.Error("CONFIRM-001 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected CONFIRM-001 on hand-rolled stdin prompt: %+v", res.Findings)
	}
}

func TestSTREAM003_IndirectWriteToStreamNamedField(t *testing.T) {
	bad := `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type services struct{ Err *os.File }
func f(r *evo.Run, s services) {
  s.Err.Write([]byte("duplicate"))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			found = true
			if f.Suggestion == "" {
				t.Error("STREAM-003 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected STREAM-003 on indirect write to stream-named field: %+v", res.Findings)
	}
}

func TestSTREAM003_NoFalsePositiveOnOrdinaryBuffer(t *testing.T) {
	good := `package p
import (
  "bytes"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  var buf bytes.Buffer
  buf.Write([]byte("fine"))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			t.Fatalf("false positive STREAM-003 on ordinary bytes.Buffer.Write: %+v", res.Findings)
		}
	}
}

func TestSTREAM003_NoFalsePositiveOnEvoOwnedWriter(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.Task) {
  task.Writer().Write([]byte("fine"))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			t.Fatalf("false positive STREAM-003 on evo-owned Writer: %+v", res.Findings)
		}
	}
}

func TestSTREAM003_NoFalsePositiveOnFprintfToBuilder(t *testing.T) {
	good := `package p
import (
  "fmt"
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  var sb strings.Builder
  fmt.Fprintf(&sb, "fine %d", 1)
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			t.Fatalf("false positive STREAM-003 on fmt.Fprintf into strings.Builder: %+v", res.Findings)
		}
	}
}

func TestSTREAM003_NoFalsePositiveOnFprintfToBuffer(t *testing.T) {
	good := `package p
import (
  "bytes"
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  var buf bytes.Buffer
  fmt.Fprintf(&buf, "fine %d", 1)
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			t.Fatalf("false positive STREAM-003 on fmt.Fprintf into bytes.Buffer: %+v", res.Findings)
		}
	}
}

func TestSTREAM003_FprintfToStdoutStillFlagged(t *testing.T) {
	bad := `package p
import (
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  fmt.Fprintf(os.Stdout, "still contaminating %d", 1)
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected STREAM-003 on fmt.Fprintf(os.Stdout, ...): %+v", res.Findings)
	}
}

func TestBOUND001_UnboundedSliceJoinIntoDetail(t *testing.T) {
	bad := `package p
import (
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(names []string) error {
  return evo.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "BOUND-001" {
			found = true
			if f.Suggestion == "" {
				t.Error("BOUND-001 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected BOUND-001 on unbounded strings.Join into Detail: %+v", res.Findings)
	}
}

func TestBOUND001_NoFalsePositiveOnBoundedSlice(t *testing.T) {
	good := `package p
import (
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(names []string) error {
  return evo.Fail("cannot delete", evo.Detail(strings.Join(names[:min(len(names), 8)], ", ")))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "BOUND-001" {
			t.Fatalf("false positive BOUND-001 when the joined slice is bounded: %+v", res.Findings)
		}
	}
}

func TestAPI030_TaskDeclaredInsideGoroutine(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(out *evo.Output, jobs []string) {
  for _, j := range jobs {
    go func(j string) {
      t := out.Task(j)
      t.Done()
    }(j)
  }
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-030" {
			found = true
			if f.Suggestion == "" {
				t.Error("API-030 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected API-030 on Task declared inside goroutine: %+v", res.Findings)
	}
}

func TestAPI030_NoFalsePositiveWhenPredeclared(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(out *evo.Output, jobs []string) {
  tasks := make([]*evo.TaskHandle, len(jobs))
  for i, j := range jobs {
    tasks[i] = out.Task(j)
  }
  for i := range jobs {
    go func(i int) {
      tasks[i].Done()
    }(i)
  }
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "API-030" {
			t.Fatalf("false positive API-030 when Task is predeclared: %+v", res.Findings)
		}
	}
}

func TestAPI031_HandRolledPhaseWriter(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type livePhase struct{ task *evo.Task }
func lastLine(p []byte) string { return string(p) }
func (w *livePhase) Write(p []byte) (int, error) {
  w.task.Doing(lastLine(p))
  return len(p), nil
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-031" {
			found = true
			if f.Suggestion == "" {
				t.Error("API-031 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected API-031 on hand-rolled phase writer: %+v", res.Findings)
	}
}

func TestAPI031_NoFalsePositiveOnOrdinaryWrite(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type buf struct{ data []byte }
func (b *buf) Write(p []byte) (int, error) {
  b.data = append(b.data, p...)
  return len(p), nil
}
func use(r *evo.Run) { _ = r }
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "API-031" {
			t.Fatalf("false positive API-031 on ordinary Write method: %+v", res.Findings)
		}
	}
}

func TestCONFIRM002_DestructiveQuestionMissingOption(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) bool {
  return r.Confirm("delete origin/production-hotfix?", evo.NonInteractive("--yes"))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "CONFIRM-002" {
			found = true
			if f.Suggestion == "" {
				t.Error("CONFIRM-002 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected CONFIRM-002 on destructive question missing Destructive(): %+v", res.Findings)
	}
}

func TestCONFIRM002_NoFalsePositiveWithDestructive(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) bool {
  return r.Confirm("delete origin/production-hotfix?", evo.Destructive())
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "CONFIRM-002" {
			t.Fatalf("false positive CONFIRM-002 when Destructive() is present: %+v", res.Findings)
		}
	}
}

func TestCONFIRM002_NoFalsePositiveOnNonDestructiveQuestion(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) bool {
  return r.Confirm("proceed?")
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "CONFIRM-002" {
			t.Fatalf("false positive CONFIRM-002 on non-destructive question: %+v", res.Findings)
		}
	}
}

func TestCON002_PrintedJoinedFailureList(t *testing.T) {
	bad := `package p
import (
  "fmt"
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, failures []string) {
  fmt.Fprintln(r.Text(), strings.Join(failures, "\n"))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "CON-002" {
			found = true
			if f.Suggestion == "" {
				t.Error("CON-002 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected CON-002 on printed joined failure list: %+v", res.Findings)
	}
}

func TestCON002_NoFalsePositiveOnPerItemResolution(t *testing.T) {
	good := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, failures []string) {
  for _, name := range failures {
    r.Task(name).Define(func(context.Context) error { return evo.Fail("failed") })
  }
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "CON-002" {
			t.Fatalf("false positive CON-002 on per-item resolution: %+v", res.Findings)
		}
	}
}

func TestCONFIRM001_NoFalsePositiveOnRunConfirm(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(r *evo.Run) bool {
  return r.Confirm("delete origin/production-hotfix?", evo.Destructive(), evo.NonInteractive("--yes"))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "CONFIRM-001" || f.RuleID == "CONFIRM-002" {
			t.Fatalf("false positive %s on r.Confirm: %+v", f.RuleID, res.Findings)
		}
	}
}
