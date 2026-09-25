package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

func TestGoSource_DetectsStartPrintfExitAndDetailMisuse(t *testing.T) {
	src := `package p
import (
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  out.Task("i").Block("b", evo.Detail(err))
  os.Exit(1)
}
`
	res := review.GoSource("x.go", src)
	want := map[string]bool{"API-006": false, "STREAM-003": false, "DOM-014": false, "API-018": false}
	for _, f := range res.Findings {
		if _, ok := want[f.RuleID]; ok {
			want[f.RuleID] = true
			if f.Line == 0 && f.RuleID == "API-006" {
				t.Errorf("API-006 missing line")
			}
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

func TestMCP014_BlockedItemAsApplicationError(t *testing.T) {
	bad := `package p
import (
  "errors"
  evo "github.com/zachbornheimer/evident-output"
)
func check() error {
  out := evo.Init(evo.Config{Title: "repo"})
  defer out.Close()
  out.Task("working tree").Block("dirty")
  return errors.New("dirty")
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
		t.Fatalf("expected DOM-011 on blocked-as-error: %+v", res.Findings)
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
}

func TestMCP014_NoFalsePositiveOnRealAppError(t *testing.T) {
	// Real evaluation failure uses Fail, not Block — must not flag DOM-011.
	good := `package p
import (
  "errors"
  evo "github.com/zachbornheimer/evident-output"
)
func check() error {
  out := evo.Init(evo.Config{Title: "repo"})
  defer out.Close()
  if err := load(); err != nil {
    out.Task("data").Fail("load failed")
    _ = out.Finish()
    return err
  }
  out.Task("data").Done()
  return out.Finish()
}
func load() error { return errors.New("io") }
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("false positive DOM-011 on Fail path: %+v", res.Findings)
		}
	}
}

func TestMCP014_BlockThenFinishOK(t *testing.T) {
	// Correct blocked path: Block + Finish, no application error return.
	ok := `package p
import evo "github.com/zachbornheimer/evident-output"
func check() error {
  out := evo.Init(evo.Config{Title: "repo"})
  defer out.Close()
  out.Task("working tree").Block("dirty")
  return out.Finish()
}
`
	res := review.GoSource("ok.go", ok)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("false positive on Finish after Block: %+v", res.Findings)
		}
	}
}

// TestDOM011_NoFalsePositiveOnCanonicalBlockInsideDefine pins the canonical
// refusal shape docs/migration/1.1.md and task.go's Block doc both name as
// the only correct form (API-081's rewrite target): inside Define,
// task.Block(...) followed by `return err` is not a Block-turned-error —
// there is no Output/Finish in scope to redirect to, and the returned error
// is what lets Define propagate the failure without overriding Block's
// resolution to Failed (E-105). DOM-011 must not fire on it.
func TestDOM011_NoFalsePositiveOnCanonicalBlockInsideDefine(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, err error) {
  task.Define(func(ctx context.Context) error {
    task.Block("refused x", evo.NextCommand("git", "status"))
    return err
  })
}
`
	res := review.GoSource("canon.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("false positive DOM-011 on canonical Block-inside-Define: %+v", f)
		}
	}
}

// TestDOM011_NoFalsePositiveOnNestedBlockInsideDefine covers the canonical
// refusal shape wrapped in a guard clause (`if err := check(); err != nil {
// task.Block(...); return err }` inside Define) — the same shape
// TestDOM011_NoFalsePositiveOnCanonicalBlockInsideDefine pins, but with an
// `if` block separating the Block call from Define's own opening brace.
// insideDefineCallback used to check only the nearest unmatched `{`, which
// is the `if` line here, not Define's — so it must walk outward through
// every enclosing scope, not stop at the first one.
func TestDOM011_NoFalsePositiveOnNestedBlockInsideDefine(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle) {
  task.Define(func(ctx context.Context) error {
    if err := check(); err != nil {
      task.Block("worktree dirty", evo.NextCommand("git", "status"))
      return err
    }
    return nil
  })
}
func check() error { return nil }
`
	res := review.GoSource("nested.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-011" {
			t.Fatalf("false positive DOM-011 on nested Block-inside-Define: %+v", f)
		}
	}
}

// TestAPI028_PrintfWithoutFormat is C6's sync: Donef and the rest of the *f
// family are deleted (Done/Summary/Task/Tasks/Changes/Plan/Warn/Reason are
// printf-variadic themselves now, and 1.1 removed Failf/Blockf too with no
// replacement in that family); Printf survives, and API-028 flags a Printf
// call with no directive at all.
func TestAPI028_PrintfWithoutFormat(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  out.Printf("modules cached")
  out.Printf("%d ok", 1)
}
`
	res := review.GoSource("x.go", src)
	var n int
	for _, f := range res.Findings {
		if f.RuleID == "API-028" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want one API-028, got %d: %+v", n, res.Findings)
	}
}

func TestAPI029_DebugWriterWarning(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.New()
  _ = out.DebugWriter()
}
`
	res := review.GoSource("x.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-029" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-029: %+v", res.Findings)
	}
}

func TestAPI026_NoFalsePositiveOnStringsMap(t *testing.T) {
	// Consumer feedback: substring ".Map(" fired on strings.Map and comments.
	src := `package p
import (
  "strings"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  // example: tasks.Map() is not real — do not flag this comment either
  slug := strings.Map(func(r rune) rune { return r }, "ABC")
  out := evo.Init(evo.Config{Title: "x"})
  t := out.Task(slug)
  t.Define(func(ctx context.Context) error {
    return nil
  })
  _ = out.Finish()
}
`
	res := review.GoSource("slug.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-026" {
			t.Fatalf("false positive API-026 on strings.Map: %+v", res.Findings)
		}
	}
	if res.Partial {
		t.Fatal("clean single-file review must not set partial=true")
	}
	if res.RecheckRequired {
		t.Fatalf("unexpected recheck: %+v", res.Findings)
	}
}

func TestAPI026_DetectsEvoExecutionHelper(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  out.Tasks("jobs").Map(func() {})
  out.Group("jobs").Retry(3)
  evo.Task("x").RunAll()
}
`
	res := review.GoSource("bad.go", src)
	var found bool
	var sawMap, sawRetry, sawRunAll bool
	for _, f := range res.Findings {
		if f.RuleID == "API-026" {
			found = true
			if f.Line == 0 {
				t.Error("API-026 missing line")
			}
			if strings.Contains(f.Message, "Map") {
				sawMap = true
			}
			if strings.Contains(f.Message, "Retry") {
				sawRetry = true
			}
			if strings.Contains(f.Message, "RunAll") {
				sawRunAll = true
			}
		}
	}
	if !found {
		t.Fatalf("expected API-026 on Tasks.Map: %+v", res.Findings)
	}
	if !sawMap || !sawRetry || !sawRunAll {
		t.Fatalf("expected API-026 on Map/Retry/RunAll, got %+v", res.Findings)
	}
}

func TestAPI026_DoesNotFlagGroupTaskDefineAfter(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(paths []string, worktrees, branches *evo.GroupHandle) {
  work := evo.Group("worktrees")
  for _, path := range paths {
    path := path
    work.Task(path).Define(func(ctx context.Context) error { return nil })
  }
  setup := evo.Sequence("setup")
  for _, path := range paths {
    path := path
    setup.Task(path).Define(func(ctx context.Context) error { return nil })
  }
  evo.Task("fetch").After(worktrees, branches).Define(func(ctx context.Context) error { return nil })
}
`
	res := review.GoSource("ok.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-026" {
			t.Fatalf("API-026 must not flag Group/Sequence/Task/Define/After: %+v", res.Findings)
		}
	}
}

func TestAPI027_CollectionDoneFail(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  g := out.Group("deps")
  g.Done()
  out.Sequence("setup").Fail("no")
  out.Group("run").Progress(1, 2)
}
`
	res := review.GoSource("col.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-027" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected API-027 on collection Done/Fail/Progress: %+v", res.Findings)
	}
}

func TestBeginnerGroupTaskDefine_NoDialectFindings(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
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
		case "API-026", "API-027", "API-032", "API-039":
			t.Fatalf("beginner file has dialect finding %s: %+v", f.RuleID, f)
		}
	}
}

func TestAPI018_AllowsMainAndExitCode(t *testing.T) {
	src := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Title: "t"})
  os.Exit(out.Run(func(o *evo.Output) error {
    o.Task("x").Done()
    return nil
  }))
}
`
	res := review.GoSource("main.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-018" {
			t.Fatalf("false positive API-018 on evo.Main: %+v", res.Findings)
		}
	}
}

// TestAPI018_AllowsExitCodeFidelityPattern pins the sanctioned pattern for
// CLIs (like go-task) that must propagate a child process's own exit code:
// capture evo.Run's code, override it when a child exits non-zero for its
// own reason, then os.Exit the resulting variable. This must not warn —
// it's a documented GoodCode shape, not something the maintainer can't
// clear (docs/guides/exit-code-fidelity.md).
func TestAPI018_AllowsExitCodeFidelityPattern(t *testing.T) {
	src := `package main
import (
  "os"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Title: "t"})
  var childErr *exec.ExitError
  code := evo.Run(func(o *evo.Output) error {
    o.Task("x").Done()
    return nil
  })
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
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Title: "t"})
  out.Task("x").Done()
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
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Title: "t"})
  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)
  go func() {
    <-c
    println("interrupted")
    os.Exit(1)
  }()
  _ = out.Finish()
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
		t.Fatalf("expected SIG-001 on signal.Notify without Cancel: %+v", res.Findings)
	}
}

func TestSIG001_NoFalsePositiveWhenCancelCalled(t *testing.T) {
	src := `package main
import (
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Title: "t"})
  t := out.Task("scan")
  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)
  go func() {
    <-c
    t.Cancel("interrupted")
  }()
  _ = out.Finish()
}
`
	res := review.GoSource("good.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "SIG-001" {
			t.Fatalf("false positive SIG-001 when Cancel is called: %+v", res.Findings)
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context) error { return nil }
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  os.Exit(evo.Main(func(context.Context) error {
    return run(ctx)
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context) error { return nil }
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  os.Exit(evo.Main(func(context.Context) error {
    return run(ctx)
  }))
}
`
	before := review.GoSource("bad.go", bad)
	if !hasRule(before, "SIG-002") {
		t.Fatalf("expected SIG-002 before remediation: %+v", before.Findings)
	}
	fixed := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(_ interface{}) error { return nil }
func main() {
  os.Exit(evo.Main(run))
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
			t.Fatalf("false positive SIG-002 with no evo.Main/Run call: %+v", res.Findings)
		}
	}
}

func TestSIG002_NoFalsePositiveWhenMainOwnsLifecycle(t *testing.T) {
	src := `package main
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context) error { return nil }
func main() {
  os.Exit(evo.Main(run))
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
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output"
)
func run() error { return nil }
func watchReload(c chan os.Signal) {}
func main() {
  reload := make(chan os.Signal, 1)
  signal.Notify(reload, syscall.SIGHUP)
  go watchReload(reload)
  os.Exit(evo.Main(func(_ interface{}) error { return run() }))
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context) error { return nil }
func main() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  os.Exit(evo.Main(func(context.Context) error {
    return run(ctx)
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output) error {
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

func TestTERM015_NoFalsePositiveWithTaskRun(t *testing.T) {
	good := `package p
import (
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output) error {
  task := out.Task("build")
  cmd := exec.Command("go", "build", "./...")
  if err := task.Run(cmd); err != nil {
    return fmt.Errorf("build failed: %w", err)
  }
  task.Done()
  return nil
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "TERM-015" {
			t.Fatalf("false positive TERM-015 when Task.Run captures the child: %+v", res.Findings)
		}
	}
}

func TestCONFIRM001_HandRolledConfirmDetected(t *testing.T) {
	bad := `package p
import (
  "bufio"
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output) error {
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
  evo "github.com/zachbornheimer/evident-output"
)
type services struct{ Err *os.File }
func f(out *evo.Output, s services) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output) {
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
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Capture().Write([]byte("fine"))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" {
			t.Fatalf("false positive STREAM-003 on evo-owned Capture writer: %+v", res.Findings)
		}
	}
}

func TestSTREAM003_NoFalsePositiveOnFprintfToBuilder(t *testing.T) {
	good := `package p
import (
  "fmt"
  "strings"
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output) {
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
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, names []string) {
  task.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))
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

func TestBOUND001_NoFalsePositiveWithTruncateNames(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, names []string) {
  task.Fail("cannot delete", evo.Detail(evo.TruncateNames(names, 8)))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "BOUND-001" {
			t.Fatalf("false positive BOUND-001 when TruncateNames bounds the text: %+v", res.Findings)
		}
	}
}

func TestAPI030_TaskDeclaredInsideGoroutine(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output"
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
import evo "github.com/zachbornheimer/evident-output"
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
import evo "github.com/zachbornheimer/evident-output"
type livePhase struct{ task *evo.TaskHandle }
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
import evo "github.com/zachbornheimer/evident-output"
type buf struct{ data []byte }
func (b *buf) Write(p []byte) (int, error) {
  b.data = append(b.data, p...)
  return len(p), nil
}
func use(out *evo.Output) { _ = out }
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
import evo "github.com/zachbornheimer/evident-output"
func f(flagYes bool) bool {
  return evo.Confirm("delete origin/production-hotfix?", evo.AssumeYes(flagYes))
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
import evo "github.com/zachbornheimer/evident-output"
func f() bool {
  return evo.Confirm("delete origin/production-hotfix?", evo.Destructive())
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
import evo "github.com/zachbornheimer/evident-output"
func f() bool {
  return evo.Confirm("proceed?")
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
  "strings"
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output, failures []string) {
  out.Println(strings.Join(failures, "\n"))
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

// TestEV001_FailEmbedsCaptureText is red-first for P7's MCP detector
// (user-13-problems.md Problem 7's named anti-pattern): a Fail summary
// built as fmt.Sprintf("install failed: %s", capture.Text()) folds the
// retained evidence ring straight into the summary, duplicating what
// auto-attach already renders as its own evidence line.
func TestEV001_FailEmbedsCaptureText(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, capture *evo.Evidence) {
  task.Fail(fmt.Sprintf("install failed: %s", capture.Text()))
}
`
	res := review.GoSource("bad.go", bad)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "EV-001" {
			found = true
			if f.Suggestion == "" {
				t.Error("EV-001 missing suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected EV-001 on Fail embedding capture.Text(): %+v", res.Findings)
	}
}

// TestEV001_NoFalsePositiveOnPavedPath proves the paved-path %w-wrapped
// error returned from Define — which lets auto-attach do its one job —
// never triggers EV-001.
func TestEV001_NoFalsePositiveOnPavedPath(t *testing.T) {
	good := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, err error) error {
  return fmt.Errorf("install dependencies: %w", err)
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "EV-001" {
			t.Fatalf("false positive EV-001 on the paved-path %%w shape: %+v", res.Findings)
		}
	}
}

func TestCON002_NoFalsePositiveOnPerItemResolution(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output, failures []string) {
  for _, name := range failures {
    out.Task(name).Fail("failed")
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

func TestFP004_PlaceholderPhaseText(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
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
		t.Fatalf("expected FP-004 on placeholder Phase text: %+v", res.Findings)
	}
}

func TestFP004_NoFalsePositiveOnDomainObjectPhase(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Doing("scanning ~/Developer/Personal/zq")
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-004" {
			t.Fatalf("false positive FP-004 on domain-object Phase text: %+v", res.Findings)
		}
	}
}

func TestFP001_HeavyIOBeforeEvoInit(t *testing.T) {
	bad := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  data, _ := os.ReadFile("config.toml")
  out := evo.Init(evo.Config{Title: "t"})
  out.Task("scan").Doing(string(data))
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
		t.Fatalf("expected FP-001 on I/O before evo.Init/New: %+v", res.Findings)
	}
}

func TestFP001_NoFalsePositiveWhenInitFirst(t *testing.T) {
	good := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  evo.Init(evo.Config{Title: "t"})
  scan := evo.Task("scan")
  scan.Doing("reading config")
  data, _ := os.ReadFile("config.toml")
  _ = data
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-001" || f.RuleID == "FP-002" {
			t.Fatalf("false positive %s when Init runs first: %+v", f.RuleID, res.Findings)
		}
	}
}

func TestFP002_InventoryBetweenIsolatedInitAndTask(t *testing.T) {
	bad := `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  return err
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
		t.Fatalf("expected FP-002 on Isolated Init then Inventory (zq purge canary): %+v", res.Findings)
	}
}

func TestFP002_NoFalsePositiveWhenTaskBeforeInventory(t *testing.T) {
	good := `package app
import evo "github.com/zachbornheimer/evident-output"
func previewPurge() error {
  planning := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "zq purge"})
  defer planning.Close()
  inv := planning.Task("inventory")
  inv.Doing("walking worktrees")
  rep, err := purge.Inventory(ctx, opt)
  _ = rep
  return err
}
`
	res := review.GoSource("purge.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-002" {
			t.Fatalf("false positive FP-002 when Task is declared before Inventory: %+v", res.Findings)
		}
	}
}

func TestFP002_HeavyIOBetweenInitAndFirstEntity(t *testing.T) {
	bad := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  evo.Init(evo.Config{Title: "t"})
  data, _ := os.ReadFile("config.toml")
  evo.Task("scan").Doing(string(data))
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
		t.Fatalf("expected FP-002 on I/O between Init and first entity: %+v", res.Findings)
	}
}

func TestFP003_PhaseSetOnceBeforeSilentSubprocess(t *testing.T) {
	bad := `package p
import evo "github.com/zachbornheimer/evident-output"
func run(t *evo.TaskHandle) {
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
		t.Fatalf("expected FP-003 on stale Phase before subprocess: %+v", res.Findings)
	}
}

func TestFP003_NoFalsePositiveWithPhaseWriter(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func run(t *evo.TaskHandle) {
  t.Doing("uploading")
  cmd.Stdout = t.Writer()
  cmd.Run()
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "FP-003" {
			t.Fatalf("false positive FP-003 when PhaseWriter wires the child: %+v", res.Findings)
		}
	}
}

func TestTAX001_HandAssembledSkipCount(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(p *evo.TaskHandle, n int) {
  p.Skipped(evo.Reason(fmt.Sprintf("%d skipped (dirty/unpushed/main)")))
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
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("protected"))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "TAX-001" {
			t.Fatalf("false positive TAX-001 on structured reason: %+v", res.Findings)
		}
	}
}

func TestPROG001_AdvanceUsage(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Advance(1)
}
`
	res := review.GoSource("x.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "PROG-001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected PROG-001 on Advance usage: %+v", res.Findings)
	}
}

func TestPROG001_PhaseStringSmugglesProgress(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, done, total int) {
  task.Doing(fmt.Sprintf("scanning %d/%d", done, total))
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
		t.Fatalf("expected PROG-001 on Phase string smuggling %%d/%%d: %+v", res.Findings)
	}
}

func TestPROG001_NoFalsePositiveOnPlainPhase(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Doing("scanning")
  task.Progress(4, 10)
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "PROG-001" {
			t.Fatalf("false positive PROG-001 on plain Phase + absolute Progress: %+v", res.Findings)
		}
	}
}

func TestCONFIRM001_NoFalsePositiveOnEvoConfirm(t *testing.T) {
	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func run(flagYes bool) bool {
  return evo.Confirm("delete origin/production-hotfix?", evo.AssumeYes(flagYes))
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "CONFIRM-001" {
			t.Fatalf("false positive CONFIRM-001 on evo.Confirm: %+v", res.Findings)
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
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output) error {
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  out.Task("i").Block("b", evo.Detail(err))
  os.Exit(1)
  out.Tasks("jobs").Map(func() {})
  out.Printf("modules cached")
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
import evo "github.com/zachbornheimer/evident-output"
func makeOut() *evo.Output { return evo.Init(evo.Config{}) }
`,
		"b.go": `package p
func use() { _ = makeOutt() }
`}))

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
		"TERM-014": true, "MCP-017": true, "API-027": true}
	src := `package p
import (
  "bufio"
  "errors"
  "fmt"
  "os"
  "os/exec"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output"
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
  out.Printf("modules cached")
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
import evo "github.com/zachbornheimer/evident-output"
func makeOut() *evo.Output { return evo.Init(evo.Config{}) }
`,
		"b.go": `package p
import "fmt"
func use() {
  out := makeOut()
  out.Task("t").Start()
  fmt.Println("x")
}
`}
	res := review.GoPackage(files)
	// Cross-file: Start and fmt from b.go must surface even though evo import is in a.go.
	var hasStart, hasStream bool
	for _, f := range res.Findings {
		if f.RuleID == "API-006" {
			hasStart = true
		}
		if f.RuleID == "STREAM-003" {
			hasStream = true
		}
	}
	if !hasStart {
		t.Fatalf("expected API-006 from cross-file Start: %+v", res.Findings)
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

func TestAPI032_NewInMain(t *testing.T) {
	// evo.New and evo.MainWith are both superseded (both removed in 1.0): Init is the constructor, Main/Output.Run are the lifecycle. Flagging a MainWith call site is required.
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
  out := evo.New(evo.Config{Title: "t"})
  evo.MainWith(out, run) // removed in 1.0 — fixture pins the detector still catches it
}
`
	res := review.GoSource("main.go", src)
	found := findAPI032(res)
	var sawNew, sawMainWith bool // sawMainWith: MainWith was removed in 1.0
	for _, f := range found {
		if strings.Contains(f.Message, "evo.New") || strings.Contains(f.Suggestion, "evo.New") {
			sawNew = true
		}
		if strings.Contains(f.Message, "MainWith") || strings.Contains(f.Suggestion, "MainWith") { // removed in 1.0
			sawMainWith = true // MainWith: removed in 1.0
			if !strings.Contains(f.Suggestion, "out.Run(run)") {
				t.Errorf("MainWith (removed in 1.0) suggestion must name out.Run(run), got %q", f.Suggestion)
			}
		}
	}
	if !sawNew {
		t.Fatalf("expected API-032 finding for evo.New in main, got %+v", found)
	}
	if !sawMainWith { // MainWith: removed in 1.0
		t.Fatalf("expected API-032 finding for evo.MainWith (removed in 1.0), got %+v", found)
	}
}

func TestAPI032_NoFalsePositiveOnHostedInstanceOutsideMain(t *testing.T) {
	// New outside main() is the documented advanced pattern (a
	// hosted-instance test harness or framework entrypoint wrapper) — must
	// not be flagged.
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
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

func TestAPI032_CauseDerivesReturnedErrorSuggestion(t *testing.T) {
	// Inside a function that returns error (a Define callback), Fail's
	// resolution can safely be dropped in favor of a returned, wrapped
	// error — Define resolves the task from that return.
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, err error) error {
  task.Fail("validate policy manifest", evo.Cause(err))
  return nil
}
`
	res := review.GoSource("cause.go", src)
	found := findAPI032(res)
	if len(found) != 1 {
		t.Fatalf("expected one API-032 finding for evo.Cause, got %+v", found)
	}
	want := `return fmt.Errorf("validate policy manifest: %w", err)`
	if found[0].Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", found[0].Suggestion, want)
	}
}

func TestAPI032_CauseOutsideErrorReturningFuncKeepsResolvingCall(t *testing.T) {
	// Outside any function that returns error, a bare `return
	// fmt.Errorf(...)` rewrite either does not compile (no error result)
	// or silently drops the Fail/Block resolution. The suggestion must
	// keep the resolving call and return the cause for the caller.
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func other(t *evo.TaskHandle, err error) {
  t.Fail("clone", evo.Cause(err))
}
`
	res := review.GoSource("cause_no_return.go", src)
	found := findAPI032(res)
	if len(found) != 1 {
		t.Fatalf("expected one API-032 finding for evo.Cause, got %+v", found)
	}
	got := found[0].Suggestion
	if strings.Contains(got, "return fmt.Errorf") {
		t.Fatalf("suggestion must not propose a return this function cannot make: %q", got)
	}
	if !strings.Contains(got, `t.Fail("clone")`) || !strings.Contains(got, "return err") {
		t.Fatalf("suggestion must keep the resolving Fail call and return err: %q", got)
	}
}

func TestAPI032_CauseOnBlockOutsideErrorReturningFuncKeepsResolvingCall(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func other(recv *evo.TaskHandle, err error) {
  recv.Block("ambiguous", evo.Cause(err))
}
`
	res := review.GoSource("cause_block_no_return.go", src)
	found := findAPI032(res)
	if len(found) != 1 {
		t.Fatalf("expected one API-032 finding for evo.Cause, got %+v", found)
	}
	got := found[0].Suggestion
	if !strings.Contains(got, `recv.Block("ambiguous")`) || !strings.Contains(got, "return err") {
		t.Fatalf("suggestion must keep the resolving Block call and return err: %q", got)
	}
}

func TestAPI032_CaptureRenamedToEvidence(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  _ = task.Capture()
}
`
	res := review.GoSource("capture.go", src)
	found := findAPI032(res)
	if len(found) != 1 {
		t.Fatalf("expected one API-032 finding for Capture, got %+v", found)
	}
	if found[0].Suggestion != "replace task.Capture(...) with task.Evidence(...)" {
		t.Fatalf("suggestion = %q", found[0].Suggestion)
	}
}

func TestAPI033_NameEqualsSkipArgument(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output, note string) {
  out.Task(note).Skip(note)
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
		t.Fatalf("expected API-033 for out.Task(note).Skip(note): %+v", res.Findings)
	}
}

func TestAPI033_NoFalsePositiveOnDistinctNameAndReason(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output, reason string) {
  out.Task("branch check").Skip(reason)
}
`
	res := review.GoSource("distinct.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-033" {
			t.Fatalf("false positive API-033 on distinct name/reason: %+v", res.Findings)
		}
	}
}

func TestAPI034_FailThenReturnNilDiscardsError(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) error {
  task := out.Task("validate")
  if err := check(); err != nil {
    task.Fail("validate failed")
    return nil
  }
  return nil
}
`
	res := review.GoSource("failnil.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-034" {
			found = true
			if f.Suggestion == "" {
				t.Fatal("expected a Suggestion")
			}
		}
	}
	if !found {
		t.Fatalf("expected API-034: %+v", res.Findings)
	}
}

func TestAPI034_NoFalsePositiveWhenErrorReturned(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output) error {
  task := out.Task("validate")
  if err := check(); err != nil {
    task.Fail("validate failed")
    return fmt.Errorf("validate failed: %w", err)
  }
  return nil
}
`
	res := review.GoSource("failreturn.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-034" {
			t.Fatalf("false positive API-034 when the error is actually returned: %+v", res.Findings)
		}
	}
}

func TestAPI035_DiscardSinkInFailingBlock(t *testing.T) {
	src := `package p
import (
  "io"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func f(out *evo.Output, cmd *exec.Cmd) {
  task := out.Task("gate")
  cmd.Stdout = io.Discard
  if err := cmd.Run(); err != nil {
    task.Block("policy check failed")
  }
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

func TestAPI035_NoFalsePositiveWithoutFailBlock(t *testing.T) {
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
			t.Fatalf("false positive API-035 with no Fail/Block in the function: %+v", res.Findings)
		}
	}
}

// TestAPI034_SprintfInVerbThenReturnNil: a Fail(fmt.Sprintf(...)) statement
// followed by a bare return nil is API-034's shape (discards the error),
// same as any other statement-form Fail.
func TestAPI034_SprintfInVerbThenReturnNil(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, branch string) error {
  task.Fail(fmt.Sprintf("delete failed on %s", branch))
  return nil
}
`
	res := review.GoSource("sprintfverb.go", src)
	var api034 int
	for _, f := range res.Findings {
		if f.RuleID == "API-034" {
			api034++
			if !strings.Contains(f.Suggestion, "return err") {
				t.Fatalf("suggestion = %q, want it to say return err", f.Suggestion)
			}
		}
	}
	if api034 != 1 {
		t.Fatalf("want one API-034, got %d: %+v", api034, res.Findings)
	}
}

// TestAPI034_BareStatementFollowedByReturnErrNotFlagged pins E-102's
// surviving half: a Block/Fail statement (Sprintf summary or not) followed
// by a returned error, not nil, already propagates the cause and is left
// alone by API-034 — that shape is the correct final form, not a finding.
func TestAPI034_BareStatementFollowedByReturnErrNotFlagged(t *testing.T) {
	src := `package p
import (
  "errors"
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, name string, n int) error {
  task.Block(fmt.Sprintf("refused %s", name))
  return errors.New("refused")
}
`
	res := review.GoSource("sprintfbare.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-034" {
			t.Fatalf("API-034 flags Block followed by a returned error, which already propagates the cause: %+v", f)
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
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, path string) {
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

// TestAPI038_WarnFlattensNotWarnf proves the Warn case flattens into Warn's
// own variadic form rather than suggesting a Warnf method that no longer
// exists (P1/P2 deleted it).
func TestAPI038_WarnFlattensNotWarnf(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, n int) {
  task.Warn(fmt.Sprintf("kept %d", n))
}
`
	res := review.GoSource("sprintfwarn.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-038" && f.Suggestion != `task.Warn("kept %d", n)` {
			t.Fatalf("suggestion = %q", f.Suggestion)
		}
	}
}

func TestAPI038_NoFalsePositiveWithoutSprintf(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, done, total int) {
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
import evo "github.com/zachbornheimer/evident-output"
type runner struct{ task *evo.TaskHandle }
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
import evo "github.com/zachbornheimer/evident-output"
type runner struct{ task *evo.TaskHandle }
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
  evo "github.com/zachbornheimer/evident-output"
)
type subject struct{ classify *evo.TaskHandle }
func (s subject) Warn(summary string, cause error) {
  s.classify.Warn(fmt.Sprintf("%s: %s", summary, cause))
}
`
	res := review.GoSource("composed.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-037" {
			t.Fatalf("false positive API-037 on a composed argument: %+v", res.Findings)
		}
	}
}

func TestDOM018_ErrTwiceViaCause(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, err error) {
  task.Fail(err.Error(), evo.Cause(err))
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
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, err, other error) {
  task.Fail(err.Error(), evo.Cause(other))
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
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, names []string) {
  task.Skipped(evo.Reason(strings.Join(names)), "x")
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
import evo "github.com/zachbornheimer/evident-output"
var reasonProtected = evo.Reason("protected")
func f(task *evo.TaskHandle) {
  task.Skipped(evo.Reason("protected"))
  task.Skipped(reasonProtected)
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
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  out.Task("copying build artifacts into the production release bucket now")
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
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) {
  out.Task("release artifacts")
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
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output, flag bool) {
  t := out.Task("scan")
  t.Doing("walking")
  if flag {
    t := out.Task("build")
    t.Done()
  }
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

func TestDOM019_NoFalsePositiveWhenResolvedFirst(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output, flag bool) {
  t := out.Task("scan")
  t.Doing("walking")
  t.Done()
  if flag {
    t := out.Task("build")
    t.Done()
  }
}
`
	res := review.GoSource("shadowclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "DOM-019" {
			t.Fatalf("false positive DOM-019 when the first handle was resolved before reassignment: %+v", res.Findings)
		}
	}
}

func TestTXT021_CrammedSummary(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Fail("policy check failed — cause: manifest missing — action: run zq init")
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
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Fail("policy check failed")
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
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output"
)
func previewPurge(roots []string) error {
  out := evo.Init(evo.Config{Isolated: true, DryRun: true})
  defer out.Close()
  for _, root := range roots {
    _ = filepath.WalkDir(root, func(string, fs.DirEntry, error) error { return nil })
  }
  out.Task("inventory").Done()
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
			if !strings.Contains(f.Suggestion, "Define") && !strings.Contains(f.Suggestion, "Each") {
				t.Fatalf("LOOP-001 suggestion must name Define/Each, got %q", f.Suggestion)
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
  "path/filepath"
  evo "github.com/zachbornheimer/evident-output"
)
func previewPurge(roots []string) error {
  out := evo.Init(evo.Config{Isolated: true, DryRun: true})
  defer out.Close()
  inv := out.Task("inventory")
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
import evo "github.com/zachbornheimer/evident-output"
func scan(paths []string) {
  out := evo.Init(evo.Config{Title: "zq"})
  worktrees := out.Group("worktrees")
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

// TestAPI082_ChainedNextAfterBlock is red-first for API-082: task.Next/
// NextCommand chained right after task.Fail/task.Block on the same handle
// should fold into the resolving call's own ProblemOption, not stay a
// separate statement — task.go documents that form as canonical.
func TestAPI082_ChainedNextAfterBlock(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Block("worktree dirty")
  task.NextCommand("git", "status")
}
`
	res := review.GoSource("chained.go", src)
	var found *review.Finding
	for i := range res.Findings {
		if res.Findings[i].RuleID == "API-082" {
			found = &res.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("expected API-082, got %+v", res.Findings)
	}
	if !strings.Contains(found.Suggestion, "evo.NextCommand") {
		t.Fatalf("suggestion = %q", found.Suggestion)
	}
}

// TestAPI082_NoFalsePositiveOnProblemOptionForm pins the fixed shape: the
// remedy already attached as a ProblemOption on Block itself must not be
// flagged again.
func TestAPI082_NoFalsePositiveOnProblemOptionForm(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
  task.Block("worktree dirty", evo.NextCommand("git", "status"))
}
`
	res := review.GoSource("noproblem.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-082" {
			t.Fatalf("false positive API-082 on the already-canonical ProblemOption form: %+v", f)
		}
	}
}
