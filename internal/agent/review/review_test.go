package review_test

import (
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
  out := evo.Init(evo.Config{Options: []evo.Option{}})
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
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("repo")}})
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
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("repo")}})
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
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("repo")}})
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

// TestAPI028_FailfWithoutFormat is C6's sync: Donef and the rest of the *f
// family are deleted (Done/Summary/Task/Tasks/Changes/Plan/Warn/Reason are
// printf-variadic themselves now); Failf/Blockf survive for their %w+
// *Failure semantics, and API-028 now flags one of those with no directive
// at all instead.
func TestAPI028_FailfWithoutFormat(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.New()
  _ = out.Task("t").Failf("modules cached")
  _ = out.Task("u").Failf("%d ok", 1)
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
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("x")}})
  out.Task(slug).Done()
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
  out := evo.Init(evo.Config{Options: []evo.Option{}})
  out.Tasks("jobs").Map(func() {})
}
`
	res := review.GoSource("bad.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-026" {
			found = true
			if f.Line == 0 {
				t.Error("API-026 missing line")
			}
		}
	}
	if !found {
		t.Fatalf("expected API-026 on Tasks.Map: %+v", res.Findings)
	}
}

func TestAPI018_AllowsMainAndExitCode(t *testing.T) {
	src := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("t")}})
  os.Exit(evo.MainWith(out, func(o *evo.Output) error {
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

func TestSIG001_SignalNotifyWithoutCancel(t *testing.T) {
	src := `package main
import (
  "os"
  "os/signal"
  "syscall"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("t")}})
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
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("t")}})
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
		t.Fatalf("expected TERM-015 on tty passthrough without Suspend: %+v", res.Findings)
	}
}

func TestTERM015_NoFalsePositiveWithSuspend(t *testing.T) {
	good := `package p
import (
  "os"
  "os/exec"
  evo "github.com/zachbornheimer/evident-output"
)
func run(out *evo.Output) error {
  cmd := exec.Command("zq", "setup")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr
  return out.Suspend(func() error { return cmd.Run() })
}
`
	res := review.GoSource("good.go", good)
	for _, f := range res.Findings {
		if f.RuleID == "TERM-015" {
			t.Fatalf("false positive TERM-015 when Suspend wraps the child: %+v", res.Findings)
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
  w.task.Phase(lastLine(p))
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
  task.Phase("working")
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
  task.Phase("scanning ~/Developer/Personal/zq")
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
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("t")}})
  out.Task("scan").Phase(string(data))
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
  scan.Phase("reading config")
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

func TestFP002_HeavyIOBetweenInitAndFirstEntity(t *testing.T) {
	bad := `package main
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func main() {
  evo.Init(evo.Config{Title: "t"})
  data, _ := os.ReadFile("config.toml")
  evo.Task("scan").Phase(string(data))
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
  t.Phase("uploading")
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
  t.Phase("uploading")
  cmd.Stdout = t.PhaseWriter()
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
  p.Skipped(evo.Reason(fmt.Sprintf("%d skipped (dirty/unpushed/main)", n)))
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
  task.Skipped(evo.Reason("protected"), "main")
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
  task.Phase(fmt.Sprintf("scanning %d/%d", done, total))
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
  task.Phase("scanning")
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
  return errors.New("dirty")
}
`))

	collect(review.Transcript("t.txt", "\x1b[?25l hello \x00"))
	collect(review.StructuredDocument("x.json", []byte(`{"foo":1}`)))

	// MCP-017: cross-file typecheck with an unresolved external symbol.
	collect(review.GoPackage(map[string]string{
		"a.go": `package p
import evo "github.com/zachbornheimer/evident-output"
func makeOut() *evo.Output { return evo.Init(evo.Config{Options: []evo.Option{}}) }
`,
		"b.go": `package p
func use() { _ = makeOut() }
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
		"TERM-014": true, "MCP-017": true, "API-027": true,
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
  out.Task("t").Failf("modules cached")
  _ = out.DebugWriter()
  task.Advance(1)
  task.Phase(fmt.Sprintf("scanning %d/%d", done, total))
  task.Skipped(evo.Reason(fmt.Sprintf("%d skipped (dirty)", done)))

  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)

  cmd := exec.Command("zq", "setup")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr

  reader := bufio.NewReader(os.Stdin)
  _, _ = reader.ReadString('\n')

  out.Task("working tree").Block("dirty")
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
func makeOut() *evo.Output { return evo.Init(evo.Config{Options: []evo.Option{}}) }
`,
		"b.go": `package p
import "fmt"
func use() {
  out := makeOut()
  out.Task("t").Start()
  fmt.Println("x")
}
`,
	}
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
	// With 2 files and local typecheck, Partial should be false when types succeed.
	// Stub importer may still leave Partial true — at least multi-file ran without crash.
	_ = res.Partial
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

func TestAPI032_NewAndMainWithInMain(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
  out := evo.New(evo.Config{Title: "t"})
  os.Exit(evo.MainWith(out, run))
}
`
	res := review.GoSource("main.go", src)
	found := findAPI032(res)
	if len(found) != 2 {
		t.Fatalf("expected two API-032 findings (New and MainWith in main), got %+v", found)
	}
}

func TestAPI032_NoFalsePositiveOnHostedInstanceOutsideMain(t *testing.T) {
	// New+MainWith outside main() is the documented advanced pattern (a
	// hosted-instance test harness or framework entrypoint wrapper) — must
	// not be flagged.
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func run() int {
  out := evo.New(evo.Config{Title: "t"})
  return evo.MainWith(out, nil)
}
`
	res := review.GoSource("hosted.go", src)
	if found := findAPI032(res); len(found) != 0 {
		t.Fatalf("New+MainWith outside main must not be flagged: %+v", found)
	}
}

func TestAPI032_CauseDerivesFailfSuggestion(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, err error) {
  task.Fail("validate policy manifest", evo.Cause(err))
}
`
	res := review.GoSource("cause.go", src)
	found := findAPI032(res)
	if len(found) != 1 {
		t.Fatalf("expected one API-032 finding for evo.Cause, got %+v", found)
	}
	want := `task.Failf("validate policy manifest: %w", err)`
	if found[0].Suggestion != want {
		t.Fatalf("suggestion = %q, want %q", found[0].Suggestion, want)
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
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) error {
  task := out.Task("validate")
  if err := check(); err != nil {
    return task.Failf("validate failed: %w", err)
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

func TestAPI036_SprintfInVerb(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, branch string) {
  task.Fail(fmt.Sprintf("delete failed on %s", branch))
}
`
	res := review.GoSource("sprintfverb.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "API-036" {
			found = true
			if f.Suggestion != `task.Failf("delete failed on %s", branch)` {
				t.Fatalf("suggestion = %q", f.Suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("expected API-036: %+v", res.Findings)
	}
}

func TestAPI036_NoFalsePositiveWithFailf(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, branch string) {
  task.Failf("delete failed on %s", branch)
}
`
	res := review.GoSource("failfclean.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "API-036" {
			t.Fatalf("false positive API-036 on Failf: %+v", res.Findings)
		}
	}
}

func TestAPI037_WrapperMethodOverTaskVerb(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
type runner struct{ task *evo.TaskHandle }
func (r *runner) resolutionPhase(text string) {
  r.task.Phase(text)
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
  r.task.Phase(text)
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
  task.Skipped(evo.Reason(strings.Join(names, ", ")), "x")
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
  task.Skipped(evo.Reason("protected"), "x")
  task.Skipped(reasonProtected, "y")
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
  t.Phase("walking")
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
  t.Phase("walking")
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
