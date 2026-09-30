package lifecycle_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func TestC02_062_MisuseIsARunLevelLineWithARemedyHint(t *testing.T) {
	out, buf := harness.New(t)
	out.Task("a").Key("same")
	out.Task("b").Key("same")
	text := harness.Text(out, buf)
	if !strings.Contains(text, "!  each task needs its own Key") {
		t.Fatalf("misuse line with remedy hint missing:\n%s", text)
	}
}

func TestC02_063_StrictConfigPanicsOnMisuse(t *testing.T) {
	out, _ := harness.New(t, func(c *evo.Config) { c.Strict = true })
	defer func() {
		if recover() == nil {
			t.Fatal("Strict did not panic on a duplicate Key")
		}
	}()
	out.Task("a").Key("same")
	out.Task("b").Key("same")
}

func TestC02_064_EachReasonCallRecordsOneTaxonomyRecord(t *testing.T) {
	out, _ := harness.New(t)
	task := out.Task("candidate")
	task.Define(func(context.Context) error { task.Skipped(evo.Reason("protected")); return nil })
	_ = task.Wait()
	if got := len(harness.MustFind(t, out, "candidate").Skipped); got != 1 {
		t.Fatalf("one Skipped(Reason) recorded %d taxonomy records", got)
	}
}
