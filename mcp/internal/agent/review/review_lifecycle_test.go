package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

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
