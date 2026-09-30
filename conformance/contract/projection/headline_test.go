package projection_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func TestC13_011_SettledRowShowsSummaryElseFirstProblem(t *testing.T) {
	out, buf := harness.New(t)
	summed := out.Task("check")
	summed.Summary("459 checked")
	_ = harness.Succeed(summed)
	failing := out.Task("lint")
	failing.Define(func(context.Context) error { return errors.New("first problem") })
	_ = failing.Wait()
	text := harness.Text(out, buf)
	if !strings.Contains(text, "✓ check  459 checked") || !strings.Contains(text, "✗ lint") || !strings.Contains(text, "first problem") {
		t.Fatalf("settled rows:\n%s", text)
	}
}
