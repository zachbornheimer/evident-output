package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
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

const sameNameVerifySrc = `package p

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func real(out *evo.Output, path string) {
	check := func(context.Context) (bool, error) {
		_, err := os.Stat(path)
		return err == nil, nil
	}
	out.Task("real").Verify(check)
}

func stamped(out *evo.Output) {
	check := func(context.Context) (bool, error) { return true, nil }
	out.Task("stamped").Verify(check)
}
`

// TestAPI063_ResolvesTheBindingInScope pins E-108: local func literals
// were keyed by name across the whole file, last binding wins, so an
// observing t.Verify(check) was flagged constant because another function
// bound a constant check — and API-063's "drop Verify" would delete a
// real postcondition. The binding is resolved in the call's own scope.
func TestAPI063_ResolvesTheBindingInScope(t *testing.T) {
	var lines []int
	for _, f := range review.GoSource("verify.go", sameNameVerifySrc).Findings {
		if f.RuleID == "API-063" {
			lines = append(lines, f.Line)
		}
	}
	if len(lines) != 1 || lines[0] != 20 {
		t.Fatalf("want API-063 only on stamped's Verify (line 20), got lines %v", lines)
	}
}
