package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

func TestGoSource_DetectsPrintfAndExitMisuse(t *testing.T) {
	src := `package p
import (
  "context"
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  fmt.Printf("hi")
  os.Exit(1)
  return nil
}
`
	res := review.GoSource("x.go", src)
	want := map[string]bool{"STREAM-003": false, "API-018": false}
	for _, f := range res.Findings {
		if _, ok := want[f.RuleID]; ok {
			want[f.RuleID] = true
		}
	}
	for id, ok := range want {
		if !ok {
			t.Errorf("missing finding %s in %#v", id, res.Findings)
		}
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
}

func TestTranscript_CursorHideShow(t *testing.T) {
	res := review.Transcript("t.txt", "\x1b[?25l hello")
	if len(res.Findings) == 0 {
		t.Fatal("expected TERM-008")
	}
}

func TestStructuredDocument_RequiresSchema(t *testing.T) {
	res := review.StructuredDocument("x.json", []byte(`{"foo":1}`))
	if !res.RecheckRequired {
		t.Fatal("expected schema findings")
	}
}

func TestDOM011_DeclinedConfirmAsApplicationError(t *testing.T) {
	bad := `package p
import (
  "context"
  "errors"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  if !r.Confirm("delete 40 branches?", evo.Destructive()) {
    return errors.New("declined")
  }
  return nil
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			found = true
			if f.Line == 0 {
				t.Error("DOM-011 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected DOM-011 on a declined Confirm returned as an error: %+v", res.Findings)
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
}

func TestDOM011_NoFalsePositiveOnRefuseOrRealError(t *testing.T) {
	good := `package p
import (
  "context"
  "errors"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  if !r.Confirm("delete 40 branches?", evo.Destructive()) {
    return evo.Refuse("declined")
  }
  if err := load(); err != nil {
    return err
  }
  return nil
}
func load() error { return errors.New("io") }
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("false positive DOM-011: %+v", res.Findings)
		}
	}
}

func TestDOM011_ConfirmOnNonEvoReceiverIsNotFlagged(t *testing.T) {
	src := `package p
import (
  "errors"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type prompter struct{}
func (prompter) Confirm(string) bool { return false }
func run(r *evo.Run, p prompter) error {
  _ = r
  if !p.Confirm("go on?") {
    return errors.New("declined")
  }
  return nil
}
`
	for _, f := range review.GoSource("other.go", src).Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("DOM-011 fired on a non-evo Confirm: %+v", f)
		}
	}
}

func TestBeginnerGroupTaskDefine_NoDialectFindings(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output/v2"
func main() {
  evo.Init(evo.Config{Title: "tool"})
  os.Exit(evo.Main(run))
}
func run() error {
  paths := []string{"a", "b"}
  worktrees := evo.Group("worktrees")
  for _, path := range paths {
    path := path
    worktrees.Task(path).Define(func(ctx context.Context) error { return check(path) })
  }
  return nil
}
func check(path string) error { return nil }
`
	res := review.GoSource("beginner.go", src)
	if res.RecheckRequired {
		t.Fatalf("beginner Group.Task+Define must have recheck_required=false, got %+v", res.Findings)
	}
	for _, f := range res.Findings {
		switch f.RuleID {
		case "API-032", "API-039":
			t.Fatalf("beginner file has dialect finding %s: %+v", f.RuleID, f)
		}
	}
}

func TestAPI018_AllowsMainAndExitCode(t *testing.T) {
	src := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  os.Exit(evo.Main(evo.Config{Name: "t"}, run))
}
`
	res := review.GoSource("main.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-018" {
			t.Fatalf("false positive API-018 on evo.Main: %+v", res.Findings)
		}
	}
}

func TestAPI018_AllowsExitCodeFidelityPattern(t *testing.T) {
	src := `package main
import (
  "os"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  var childErr *exec.ExitError
  code := evo.Main(evo.Config{Name: "t"}, run)
  if childErr != nil {
    code = childErr.ExitCode()
  }
  os.Exit(code)
}
`
	res := review.GoSource("main.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-018" {
			t.Fatalf("false positive API-018 on the exit-code-fidelity pattern: %+v", res.Findings)
		}
	}
}

// TestAPI018_StillFlagsNakedExit proves the fidelity-pattern allowance
// above did not weaken detection of a genuinely bare os.Exit(1) that never
// touched evo.Run's returned code.
func TestAPI018_StillFlagsNakedExit(t *testing.T) {
	src := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  code := evo.Main(evo.Config{Name: "t"}, run)
  _ = code
  os.Exit(1)
}
`
	res := review.GoSource("main.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-018" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-018 on naked os.Exit(1): %+v", res.Findings)
	}
}

func TestSIG001_SignalNotifyWithoutCancel(t *testing.T) {
	src := `package main
import (
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)
  go func() {
    <-c
    println("interrupted")
    os.Exit(1)
  }()
  os.Exit(evo.Main(evo.Config{Name: "t"}, run))
}
`
	res := review.GoSource("bad.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "SIG-001" {
			found = true
			if f.Line == 0 {
				t.Error("SIG-001 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected SIG-001 on a signal handler that calls os.Exit: %+v", res.Findings)
	}
}

func TestSIG001_NoFalsePositiveWhenHandlerCancelsTheContext(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  ctx, cancel := context.WithCancel(context.Background())
  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)
  go func() {
    <-c
    cancel()
  }()
  os.Exit(evo.Execute(ctx, evo.Config{Name: "t"}, run).ExitCode())
}
`
	res := review.GoSource("good.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "SIG-001" {
			t.Fatalf("false positive SIG-001 when the handler cancels the context: %+v", res.Findings)
		}
	}
}

func TestSIG002_DuplicateSignalWiringAroundMain(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error { return nil }
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  os.Exit(evo.Main(evo.Config{}, func(_ context.Context, r *evo.Run) error {
    return run(ctx, r)
  }))
}
`
	res := review.GoSource("bad.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "SIG-002" {
			found = true
			if f.Line == 0 {
				t.Error("SIG-002 missing line")
			}
			if f.Suggestion == "" {
				t.Error("SIG-002 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected SIG-002 on signal.NotifyContext duplicating evo.Main's SIGINT/SIGTERM lifecycle: %+v", res.Findings)
	}
}

func TestSIG002_RecheckClearsAfterRemediation(t *testing.T) {
	bad := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error { return nil }
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  os.Exit(evo.Main(evo.Config{}, func(_ context.Context, r *evo.Run) error {
    return run(ctx, r)
  }))
}
`
	before := review.GoSource("bad.go", bad)
	if !hasRule(before, "SIG-002") {
		t.Fatalf("expected SIG-002 before remediation: %+v", before.Findings)
	}
	fixed := `package main
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error { return nil }
func main() {
  os.Exit(evo.Main(evo.Config{}, run))
}
`
	after := review.GoSource("fixed.go", fixed)
	if hasRule(after, "SIG-002") {
		t.Fatalf("SIG-002 must be gone after deleting the duplicate signal.NotifyContext layer: %+v", after.Findings)
	}
}

func TestSIG002_NoFalsePositiveWithoutMainOrRun(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
)
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  _ = ctx
}
`
	res := review.GoSource("good_no_evo.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "SIG-002" {
			t.Fatalf("false positive SIG-002 with no evo.Main call: %+v", res.Findings)
		}
	}
}

func TestSIG002_NoFalsePositiveWhenMainOwnsLifecycle(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error { return nil }
func main() {
  os.Exit(evo.Main(evo.Config{}, run))
}
`
	res := review.GoSource("good.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "SIG-002" {
			t.Fatalf("false positive SIG-002 when evo.Main owns the whole lifecycle: %+v", res.Findings)
		}
	}
}

func TestSIG002_NoFalsePositiveOnUnrelatedSignal(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error { return nil }
func watchReload(c chan os.Signal) {}
func main() {
  reload := make(chan os.Signal, 1)
  signal.Notify(reload, syscall.SIGHUP)
  go watchReload(reload)
  os.Exit(evo.Main(evo.Config{}, run))
}
`
	res := review.GoSource("good_sighup.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "SIG-002" {
			t.Fatalf("false positive SIG-002 on unrelated SIGHUP handling: %+v", res.Findings)
		}
	}
}

func TestSIG002_NotFlaggedBeforeDialectOneZero(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error { return nil }
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  os.Exit(evo.Main(evo.Config{}, func(_ context.Context, r *evo.Run) error {
    return run(ctx, r)
  }))
}
`
	res := review.GoSourceAt("bad.go", src, "v0.4.7")
	for _, f := range res.Findings {
		if f.RuleID == "SIG-002" {
			t.Fatalf("SIG-002 must not fire for a pin before evo.Main owned SIGINT/SIGTERM (1.0.0): %+v", res.Findings)
		}
	}
}

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

// TestReviewEmittedIDsAreRegistered exercises every review detector against a
// real trigger and asserts the resulting rule IDs all resolve through
// rules.Explain — an ID review can emit but rules.Explain can't resolve is
// exactly the agent-loop dead end this test exists to close.
func TestReviewEmittedIDsAreRegistered(t *testing.T) {
	emitted := map[string]bool{}

	collect := func(res review.Result) {
		for _, f := range res.Findings {
			emitted[f.RuleID] = true
		}
	}

	collect(review.GoSource("parse-error.go", `package p
func f( {
`))

	collect(review.GoSource("all-findings.go", `package p
import (
  "bufio"
  "errors"
  "fmt"
  "os"
  "os/exec"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(out *evo.Output) error {
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  out.Task("i").Block("b", evo.Detail(err))
  os.Exit(1)
  out.Tasks("jobs").Map(func() {})
  out.Task("t").Failf("modules cached")
  _ = out.DebugWriter()

  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)

  cmd := exec.Command("zq", "setup")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr

  reader := bufio.NewReader(os.Stdin)
  _, _ = reader.ReadString('\n')

  out.Task("working tree").Block("dirty")
  out.Task("go@1.25.11").Done("/bin/go")
  jobs := out.Group("run")
  jobs.Task("install:fresh-start")
  return errors.New("dirty")
}
`))

	collect(review.Transcript("t.txt", "\x1b[?25l hello \x00"))
	collect(review.StructuredDocument("x.json", []byte(`{"foo":1}`)))

	// MCP-017: cross-file typecheck with an unresolved local symbol.
	collect(review.GoPackage(map[string]string{
		"a.go": `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func makeOut() *evo.Output { return evo.Init(evo.Config{}) }
`,
		"b.go": `package p
func use() { _ = makeOutt() }
`,
	}))

	if len(emitted) == 0 {
		t.Fatal("no findings collected; fixtures no longer trigger any rule")
	}
	for id := range emitted {
		if id == "" {
			continue
		}
		if _, ok := rules.Explain(id); !ok {
			t.Errorf("review emits %s but rules.Explain(%q) cannot resolve it", id, id)
		}
	}
}

// TestCallSiteFindingsCarrySuggestion proves every call-site detector finding
// (as opposed to structural findings like a parse error or missing schema
// field, which have no call site to substitute into) names a concrete,
// identifier-substituted fix — the steer that a finding must be actionable,
// not just a citation of the rule it violates.
func TestCallSiteFindingsCarrySuggestion(t *testing.T) {
	structural := map[string]bool{
		"API-000": true, "SCHEMA-001": true, "TERM-008": true,
		"TERM-014": true, "MCP-017": true,
	}
	src := `package p
import (
  "bufio"
  "errors"
  "fmt"
  "os"
  "os/exec"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type services struct{ Err *os.File }
func run(out *evo.Output, svc services, task *evo.TaskHandle, done, total int) error {
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  svc.Err.Write([]byte("dup"))
  out.Task("i").Block("b", evo.Detail(err))
  os.Exit(1)
  out.Tasks("jobs").Map(func() {})
  out.Task("t").Failf("modules cached")
  _ = out.DebugWriter()
  task.Advance(1)
  task.Doing(fmt.Sprintf("scanning %d/%d", done, total))
  task.Skipped(evo.Reason(fmt.Sprintf("%d skipped (dirty)")))

  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)

  cmd := exec.Command("zq", "setup")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr

  reader := bufio.NewReader(os.Stdin)
  _, _ = reader.ReadString('\n')

  out.Task("working tree").Block("dirty")
  out.Task("go@1.25.11").Done("/bin/go")
  jobs := out.Group("run")
  jobs.Task("install:fresh-start")
  return errors.New("dirty")
}
`
	res := review.GoSource("x.go", src)
	if len(res.Findings) == 0 {
		t.Fatal("fixture produced no findings")
	}
	for _, f := range res.Findings {
		if structural[f.RuleID] {
			continue
		}
		if f.Suggestion == "" {
			t.Errorf("%s at line %d has no Suggestion: %+v", f.RuleID, f.Line, f)
		}
	}
}

func TestGoPackage_CrossFileTypes(t *testing.T) {
	// MCP-017: two files, shared package — review resolves across files.
	files := map[string]string{
		"a.go": `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func declare(r *evo.Run) *evo.Task { return r.Task("t") }
`,
		"b.go": `package p
import "fmt"
func use() {
  fmt.Println("x")
}
`,
	}
	res := review.GoPackage(files)
	// Cross-file: fmt from b.go must surface even though the evo import is in a.go.
	var hasStream bool
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			hasStream = true
		}
	}
	if !hasStream {
		t.Fatalf("expected STREAM-003 from cross-file fmt: %+v", res.Findings)
	}
	if res.Partial {
		t.Fatalf("both files type-check locally, so the review is not partial: %+v", res.Findings)
	}
}

func findAPI032(res review.Result) []review.Finding {
	var found []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-032" {
			found = append(found, f)
		}
	}
	return found
}

func TestAPI032_NoFalsePositiveOnHostedInstanceOutsideMain(t *testing.T) {
	// New outside main() is the documented advanced pattern (a
	// hosted-instance test harness or framework entrypoint wrapper) — must
	// not be flagged.
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run() int {
  out := evo.New(evo.Config{Title: "t"})
  return out.Run(nil)
}
`
	res := review.GoSource("hosted.go", src)
	if found := findAPI032(res); len(found) != 0 {
		t.Fatalf("New outside main must not be flagged: %+v", found)
	}
}

func TestAPI033_NameEqualsExcludeArgument(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, note string) {
  r.Task(note).Define(func(context.Context) error { return evo.Exclude(note) })
}
`
	res := review.GoSource("dup.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-033" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-033 for Task(note) returning Exclude(note): %+v", res.Findings)
	}
}

func TestAPI033_NoFalsePositiveOnDistinctNameAndReason(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, reason string) {
  r.Task("branch check").Define(func(context.Context) error { return evo.Exclude(reason) })
}
`
	res := review.GoSource("distinct.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-033" {
			t.Fatalf("false positive API-033 on distinct name/reason: %+v", res.Findings)
		}
	}
}

func TestAPI035_DiscardSinkInFailingBlock(t *testing.T) {
	src := `package p
import (
  "io"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(cmd *exec.Cmd) error {
  cmd.Stdout = io.Discard
  if err := cmd.Run(); err != nil {
    return evo.Refuse("policy check failed")
  }
  return nil
}
`
	res := review.GoSource("discard.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-035" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-035: %+v", res.Findings)
	}
}

func TestAPI035_NoFalsePositiveWithoutFailRefuse(t *testing.T) {
	src := `package p
import (
  "io"
  "os/exec"
)
func f(cmd *exec.Cmd) {
  cmd.Stdout = io.Discard
  _ = cmd.Run()
}
`
	res := review.GoSource("discardclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-035" {
			t.Fatalf("false positive API-035 with no Fail/Refuse in the function: %+v", res.Findings)
		}
	}
}

// TestAPI038_SprintfIntoVariadicVerb is red-first for the new detector: a
// printf-variadic evo method (Doing here) already accepts format + args
// directly, so wrapping the call in fmt.Sprintf is ceremony to flatten away.
func TestAPI038_SprintfIntoVariadicVerb(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(task *evo.Task, path string) {
  task.Doing(fmt.Sprintf("scanning %s", path))
}
`
	res := review.GoSource("sprintfdoing.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-038" {
			found = true
			if f.Suggestion != `task.Doing("scanning %s", path)` {
				t.Fatalf("suggestion = %q", f.Suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("expected API-038: %+v", res.Findings)
	}
}

// TestAPI038_WarnMigratesToProblem proves dirty task.Warn(fmt.Sprintf(...))
// rewrites through API-032 to Task.Problem. Warn is removed
// in 1.1: do not flatten the Sprintf into task.Warn, and do not invent Warnf.
func TestAPI038_WarnMigratesToProblem(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(task *evo.TaskHandle, n int) {
  task.Warn(fmt.Sprintf("kept %d", n))
}
`
	res := review.GoSource("sprintfwarn.go", src)
	var api032 review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-038" {
			t.Fatalf("API-038 must not flatten Warn; migrate to Problem: %+v", f)
		}
		if f.RuleID == "API-032" && strings.Contains(f.Suggestion, "Problem(") {
			api032 = f
		}
	}
	if api032.Suggestion == "" {
		t.Fatalf("expected API-032 Problem rewrite, got %+v", res.Findings)
	}
	if !strings.Contains(api032.Suggestion, `task.Problem(fmt.Sprintf("kept %d", n))`) {
		t.Fatalf("suggestion = %q", api032.Suggestion)
	}
}

func TestAPI038_NoFalsePositiveWithoutSprintf(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(task *evo.Task, done, total int) {
  task.Doing("scanning %d/%d", done, total)
}
`
	res := review.GoSource("doingclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-038" {
			t.Fatalf("false positive API-038 on an already-flattened call: %+v", res.Findings)
		}
	}
}

func TestAPI037_WrapperMethodOverTaskVerb(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type runner struct{ task *evo.Task }
func (r *runner) resolutionPhase(text string) {
  r.task.Doing(text)
}
`
	res := review.GoSource("wrapper.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-037: %+v", res.Findings)
	}
}

func TestAPI037_NoFalsePositiveOnMultiStatementMethod(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type runner struct{ task *evo.Task }
func (r *runner) resolutionPhase(text string) {
  r.task.Doing(text)
  r.task.Progress(1, 2)
}
`
	res := review.GoSource("wrapperclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			t.Fatalf("false positive API-037 on a multi-statement method: %+v", res.Findings)
		}
	}
}

// A wrapper that composes the verb's argument is not a bare passthrough:
// the caller cannot "inline the verb" without copying that composition, so
// the name and the frame are earning their place.
func TestAPI037_NoFalsePositiveOnComposedArgument(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type subject struct{ classify *evo.Task }
func (s subject) Problem(summary string, cause error) {
  s.classify.Problem(fmt.Sprintf("%s: %s", summary, cause))
}
`
	res := review.GoSource("composed.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			t.Fatalf("false positive API-037 on a composed argument: %+v", res.Findings)
		}
	}
}

func TestDOM018_ErrTwiceViaDetail(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(err error) error {
  return evo.Fail(err.Error(), evo.Detail(err.Error()))
}
`
	res := review.GoSource("errtwice.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "DOM-018" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DOM-018: %+v", res.Findings)
	}
}

func TestDOM018_NoFalsePositiveOnDistinctErrorVars(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(err, other error) error {
  return evo.Fail(err.Error(), evo.Detail(other.Error()))
}
`
	res := review.GoSource("errdistinct.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-018" {
			t.Fatalf("false positive DOM-018 on distinct error variables: %+v", res.Findings)
		}
	}
}

func TestTAX002_DynamicReasonFromJoin(t *testing.T) {
	src := `package p
import (
  "strings"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(names []string) error {
  return evo.Exclude(strings.Join(names, ", "))
}
`
	res := review.GoSource("dynreason.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TAX-002" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TAX-002: %+v", res.Findings)
	}
}

func TestTAX002_NoFalsePositiveOnLiteralOrPackageVar(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
const reasonProtected = "protected"
func f(literal bool) error {
  if literal {
    return evo.Exclude("protected")
  }
  return evo.Exclude(reasonProtected)
}
`
	res := review.GoSource("reasonclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "TAX-002" {
			t.Fatalf("false positive TAX-002 on a literal/package-var reason: %+v", res.Findings)
		}
	}
}

func TestTXT020_LongEntityName(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("copying build artifacts into the production release bucket now")
}
`
	res := review.GoSource("longname.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TXT-020" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TXT-020: %+v", res.Findings)
	}
}

func TestTXT020_NoFalsePositiveOnShortNoun(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("release artifacts")
}
`
	res := review.GoSource("shortname.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "TXT-020" {
			t.Fatalf("false positive TXT-020 on a short noun name: %+v", res.Findings)
		}
	}
}

func TestDOM019_ShadowedHandleBeforeResolution(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  t := r.Task("scan")
  t.Doing("walking")
  t = r.Task("build")
  t.Define(build)
}
`
	res := review.GoSource("shadow.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "DOM-019" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected DOM-019: %+v", res.Findings)
	}
}

func TestDOM019_NoFalsePositiveWhenDefinedFirst(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run, flag bool) {
  t := r.Task("scan")
  t.Doing("walking")
  t.Define(scan)
  t = r.Task("build")
  t.Define(build)
  if flag {
    t := r.Task("publish")
    t.Define(publish)
  }
}
`
	res := review.GoSource("shadowclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-019" {
			t.Fatalf("false positive DOM-019 when the first Task was Defined before reassignment: %+v", res.Findings)
		}
	}
}

func TestTXT021_CrammedSummary(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("policy").Define(func(ctx context.Context) error {
    return evo.Fail("policy check failed — cause: manifest missing — action: run zq init")
  })
}
`
	res := review.GoSource("crammed.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "TXT-021" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TXT-021: %+v", res.Findings)
	}
}

func TestTXT021_NoFalsePositiveOnPlainSummary(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("policy").Define(func(ctx context.Context) error {
    return evo.Fail("policy check failed", evo.Detail("manifest missing"))
  })
}
`
	res := review.GoSource("plainsummary.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "TXT-021" {
			t.Fatalf("false positive TXT-021 on a plain summary: %+v", res.Findings)
		}
	}
}

// TestDOM020_GuidanceOnlyResolvesButIsNeverEmitted pins the exit-code
// honesty rule as guidance-only: rules.Explain resolves it (so a guide
// reference is never a dead end), but no fixture can make review.GoSource
// emit it — telling a usage mistake from a genuine evaluation failure needs
// the caller's own domain judgment, not a source pattern.
func TestDOM020_GuidanceOnlyResolvesButIsNeverEmitted(t *testing.T) {
	r, ok := rules.Explain("DOM-020")
	if !ok {
		t.Fatal("rules.Explain(\"DOM-020\") did not resolve")
	}
	if r.Detection != "guidance" {
		t.Fatalf("Detection = %q, want %q", r.Detection, "guidance")
	}
}

func TestLOOP001_SilentForRangeBeforeTask(t *testing.T) {
	bad := `package app
import (
  "context"
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run, roots []string) error {
  for _, root := range roots {
    _ = filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil })
  }
  r.Task("inventory").Define(func(ctx context.Context) error { return nil })
  return nil
}
`
	res := review.GoSource("purge_loop.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "LOOP-001" {
			found = true
			if f.Line == 0 {
				t.Error("LOOP-001 missing line")
			}
			if !strings.Contains(f.Suggestion, "Define") {
				t.Fatalf("LOOP-001 suggestion must name Define, got %q", f.Suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("expected LOOP-001 on for/range WalkDir before Task: %+v", res.Findings)
	}
}

func TestLOOP001_NoFalsePositiveWhenLoopInsideDefine(t *testing.T) {
	good := `package app
import (
  "context"
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func previewPurge(ctx context.Context, r *evo.Run, roots []string) error {
  inv := r.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func(ctx context.Context) error {
    for _, root := range roots {
      if err := filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil }); err != nil {
        return err
      }
    }
    return nil
  })
  return nil
}
`
	res := review.GoSource("purge_loop_ok.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "LOOP-001" || f.RuleID == "FP-002" {
			t.Fatalf("false positive %s when Task/Doing/Define runs before the loop: %+v", f.RuleID, res.Findings)
		}
	}
}

func TestLOOP001_NoFalsePositiveOnGroupTaskPerItem(t *testing.T) {
	good := `package app
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func scan(ctx context.Context, r *evo.Run, paths []string) {
  worktrees := r.Group("worktrees")
  for _, path := range paths {
    path := path
    worktrees.Task(path).Define(func(ctx context.Context) error { return filepath.WalkDir(path, nil) })
  }
}
`
	res := review.GoSource("group_task_ok.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "LOOP-001" {
			t.Fatalf("false positive LOOP-001 on Group + one Task per item: %+v", res.Findings)
		}
	}
}
