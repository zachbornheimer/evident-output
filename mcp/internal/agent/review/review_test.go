package review_test

import (
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
