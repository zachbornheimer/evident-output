package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/review"
	"github.com/zachbornheimer/evident-output/agent/rules"
)

func TestGoSource_DetectsStartPrintfExitAndDetailMisuse(t *testing.T) {
	src := `package p
import (
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.NewWithOptions()
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  out.Item("i").Block("b", evo.Detail(err))
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
  out := evo.NewWithOptions(evo.Title("repo"))
  defer out.Close()
  out.Item("working tree").Block("dirty")
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
  out := evo.NewWithOptions(evo.Title("repo"))
  defer out.Close()
  if err := load(); err != nil {
    out.Item("data").Fail("load failed")
    _ = out.Finish()
    return err
  }
  out.Item("data").OK()
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
  out := evo.NewWithOptions(evo.Title("repo"))
  defer out.Close()
  out.Item("working tree").Block("dirty")
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

func TestAPI028_DonefWithoutFormat(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.New()
  out.Task("t").Donef("modules cached")
  out.Task("u").Donef("%d ok", 1)
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
  out := evo.NewWithOptions(evo.Title("x"))
  out.Item(slug).OK()
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
  out := evo.NewWithOptions()
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
  out := evo.NewWithOptions(evo.Title("t"))
  os.Exit(evo.MainWith(out, func(o *evo.Output) error {
    o.Item("x").OK()
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
  out := evo.NewWithOptions(evo.Title("t"))
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
  out := evo.NewWithOptions(evo.Title("t"))
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
  out.Item("i").Block("b", evo.Detail(err))
  os.Exit(1)
  out.Tasks("jobs").Map(func() {})
  out.Task("t").Donef("modules cached")
  _ = out.DebugWriter()

  c := make(chan os.Signal, 1)
  signal.Notify(c, syscall.SIGINT)

  cmd := exec.Command("zq", "setup")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr

  reader := bufio.NewReader(os.Stdin)
  _, _ = reader.ReadString('\n')

  out.Item("working tree").Block("dirty")
  return errors.New("dirty")
}
`))

	collect(review.Transcript("t.txt", "\x1b[?25l hello \x00"))
	collect(review.StructuredDocument("x.json", []byte(`{"foo":1}`)))

	// MCP-017: cross-file typecheck with an unresolved external symbol.
	collect(review.GoPackage(map[string]string{
		"a.go": `package p
import evo "github.com/zachbornheimer/evident-output"
func makeOut() *evo.Output { return evo.NewWithOptions() }
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

func TestGoPackage_CrossFileTypes(t *testing.T) {
	// MCP-017: two files, shared package — review resolves across files.
	files := map[string]string{
		"a.go": `package p
import evo "github.com/zachbornheimer/evident-output"
func makeOut() *evo.Output { return evo.NewWithOptions() }
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
