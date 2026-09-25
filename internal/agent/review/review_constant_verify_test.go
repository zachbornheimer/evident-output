package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const constantVerifySrc = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	out.Task("setup").Verify(func(context.Context) (bool, error) { return true, nil }).Define(apply)
	check := func(context.Context) (bool, error) { return false, nil }
	out.Task("other").Verify(check)
}
`

// TestAPI063_ConstantVerify_Fires pins E-099: a Verify callback that
// returns a constant claims already-satisfied (or never satisfied)
// without observing anything: the retired Done stamp under another name.
func TestAPI063_ConstantVerify_Fires(t *testing.T) {
	res := review.GoSource("verify.go", constantVerifySrc)
	n := 0
	for _, f := range res.Findings {
		if f.RuleID == "API-063" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("want API-063 on both constant Verify callbacks (inline and bound), got %d: %+v", n, res.Findings)
	}
}

const observingVerifySrc = `package p

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output, path string) {
	out.Task("setup").Verify(func(context.Context) (bool, error) {
		_, err := os.Stat(path)
		return err == nil, nil
	}).Define(apply)
}
`

func TestAPI063_ObservingVerify_Silent(t *testing.T) {
	for _, f := range review.GoSource("verify.go", observingVerifySrc).Findings {
		if f.RuleID == "API-063" {
			t.Fatalf("API-063 on a Verify that observes state: %+v", f)
		}
	}
}
