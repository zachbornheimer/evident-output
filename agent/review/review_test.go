package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/review"
)

func TestGoSource_DetectsStartAndPrintf(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.New()
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
}
`
	res := review.GoSource("x.go", src)
	var sawAPI, sawStream bool
	for _, f := range res.Findings {
		if f.RuleID == "API-006" {
			sawAPI = true
		}
		if f.RuleID == "STREAM-003" {
			sawStream = true
		}
	}
	if !sawAPI || !sawStream {
		t.Fatalf("findings=%#v", res.Findings)
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
}
