package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

func TestCallSoup_ExtraPositionalArgsAtTaskIsDirty(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  out := evo.Init(evo.Config{Title: "t"})
  _ = out.Task("scan", "extra")
}
`
	res := review.GoSource("soup.go", src)
	var found bool
	for _, f := range res.Findings {
		if f.RuleID != "API-032" {
			continue
		}
		found = true
		if f.Line == 0 {
			t.Error("API-032 missing line")
		}
	}
	if !found {
		t.Fatalf("expected API-032 on extra Task args: %+v", res.Findings)
	}
}

func TestCALL001_InlineMakeInsideInitArgIsDirty(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  out := evo.Init(evo.Config{Title: "t", Facts: make([]evo.Fact, 0)})
  _ = out.Task("scan")
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
		t.Fatalf("expected CALL-001 on inline make inside Init: %+v", res.Findings)
	}
}

func TestCALL001_InlineNewInsideInitArgIsDirty(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  out := evo.Init(evo.Config{Title: "t", Clock: new(int)})
  _ = out.Group("jobs")
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
		t.Fatalf("expected CALL-001 on inline new inside Init: %+v", res.Findings)
	}
}

func TestCALL001_NamedLocalsExtractedBeforeCallAreClean(t *testing.T) {
	src := `package app
import evo "github.com/zachbornheimer/evident-output"
func run() {
  facts := make([]evo.Fact, 0)
  n := new(int)
  _ = n
  cfg := evo.Config{Title: "t", Facts: facts}
  out := evo.Init(cfg)
  _ = out.Task("scan")
  _ = out.Group("jobs")
}
`
	res := review.GoSource("named_local.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "CALL-001" || f.RuleID == "API-032" {
			t.Fatalf("named locals before evo.Init/Task/Group must be clean: %+v", res.Findings)
		}
	}
}
