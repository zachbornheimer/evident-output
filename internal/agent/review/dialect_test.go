package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const newSrc = `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
	_ = evo.New(evo.Config{Title: "t"})
}
`

func TestGoSource_NewIsAPI032AtCurrentDialectNotPre04(t *testing.T) {
	current := review.GoSource("main.go", newSrc)
	if !hasRule(current, "API-032") {
		t.Fatalf("current dialect must flag evo.New, got %+v", current.Findings)
	}
	old := review.GoSourceAt("main.go", newSrc, "v0.2.9")
	if hasRule(old, "API-032") {
		t.Fatalf("pre-0.4 desired_version must not flag evo.New, got %+v", old.Findings)
	}
	if old.DesiredVersion != "v0.2.9" {
		t.Fatalf("DesiredVersion=%q", old.DesiredVersion)
	}
}

func TestGoSource_ApplyInitSuggestionClearsAPI032(t *testing.T) {
	res := review.GoSource("main.go", newSrc)
	var sug string
	for _, f := range res.Findings {
		if f.RuleID == "API-032" && strings.Contains(f.Message, "evo.New") {
			sug = f.Suggestion
			break
		}
	}
	if sug == "" {
		t.Fatalf("missing New suggestion: %+v", res.Findings)
	}
	fixed := strings.Replace(newSrc, "evo.New(evo.Config{Title: \"t\"})", "evo.Init(evo.Config{Title: \"t\"})", 1)
	if !strings.Contains(sug, "evo.Init") {
		t.Fatalf("suggestion must name evo.Init, got %q", sug)
	}
	again := review.GoSource("main.go", fixed)
	if hasRule(again, "API-032") {
		t.Fatalf("after applying Init, API-032 must be gone, got %+v", again.Findings)
	}
	if again.RecheckRequired {
		t.Fatal("RecheckRequired must be false after the New→Init fix")
	}
}

func TestGoSource_PolicyPlanAndRunnerCaptureAreNotAPI032(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(policy struct{ Plan func() }, runner struct{ Capture func() string }, out *evo.Output) {
	_ = policy.Plan()
	_ = runner.Capture()
	out.Plan("x")
}
`
	res := review.GoSource("x.go", src)
	var sawEvoPlan, sawPolicy, sawCapture bool
	for _, f := range res.Findings {
		if f.RuleID != "API-032" {
			continue
		}
		if strings.Contains(f.Suggestion, "out.Plan") || strings.Contains(f.Message, "Plan was removed") {
			sawEvoPlan = true
		}
		if strings.Contains(f.Suggestion, "policy.Plan") {
			sawPolicy = true
		}
		if strings.Contains(f.Suggestion, "runner.Capture") {
			sawCapture = true
		}
	}
	if !sawEvoPlan {
		t.Fatalf("out.Plan must still be API-032, got %+v", res.Findings)
	}
	if sawPolicy || sawCapture {
		t.Fatalf("policy.Plan / runner.Capture must not be API-032, got %+v", res.Findings)
	}
}

func hasRule(res review.Result, id string) bool {
	for _, f := range res.Findings {
		if f.RuleID == id {
			return true
		}
	}
	return false
}
