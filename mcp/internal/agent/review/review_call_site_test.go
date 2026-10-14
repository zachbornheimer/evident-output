package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

func TestCALL001_InlineMakeInsideMainArgIsDirty(t *testing.T) {
	src := `package app
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func main() {
  os.Exit(evo.Main(evo.Config{Facades: evo.Facades{Stdout: make(chan int)}}, run))
}
`
	res := review.GoSource("inline_make.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID != "CALL-001" {
			continue
		}
		found = true
		if f.Line == 0 {
			t.Error("CALL-001 missing line")
		}
		if !strings.Contains(f.Suggestion, "make") && !strings.Contains(f.Message, "make") {
			t.Fatalf("CALL-001 must name make/new, got message=%q suggestion=%q", f.Message, f.Suggestion)
		}
	}
	if !found {
		t.Fatalf("expected CALL-001 on inline make inside evo.Main: %+v", res.Findings)
	}
}

func TestCALL001_InlineNewInsideExecuteArgIsDirty(t *testing.T) {
	src := `package app
import (
  "bytes"
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func capture(ctx context.Context) int {
  return evo.Execute(ctx, evo.Config{Facades: evo.Facades{Stdout: new(bytes.Buffer)}}, run).ExitCode()
}
`
	res := review.GoSource("inline_new.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID == "CALL-001" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected CALL-001 on inline new inside evo.Execute: %+v", res.Findings)
	}
}

func TestCALL001_NamedLocalsExtractedBeforeCallAreClean(t *testing.T) {
	src := `package app
import (
  "bytes"
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func capture(ctx context.Context) int {
  stdout := new(bytes.Buffer)
  cfg := evo.Config{Facades: evo.Facades{Stdout: stdout}}
  return evo.Execute(ctx, cfg, run).ExitCode()
}
`
	res := review.GoSource("named_local.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "CALL-001" {
			t.Fatalf("named locals before evo.Execute must be clean: %+v", res.Findings)
		}
	}
}
