package projection_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func skipTask(parent interface {
	Task(string) *evo.TaskHandle
}, name, reason string) {
	task := parent.Task(name)
	task.Define(func(context.Context) error { task.Skipped(evo.Reason(reason)); return nil })
}

func TestC13_001_SkippedChildrenFoldIntoOnePerReasonTally(t *testing.T) {
	out, buf := harness.New(t)
	g := out.Group("branches")
	own := g.Task("branches")
	own.Summary("2 checked")
	own.Define(func(context.Context) error { return nil })
	skipTask(g, "b1", "checked out")
	skipTask(g, "b2", "checked out")
	skipTask(g, "b3", "protected")
	_ = g.Wait()
	text := harness.Text(out, buf)
	if !strings.Contains(text, "- skipped 3 (2 checked out, 1 protected)") {
		t.Fatalf("tally missing:\n%s", text)
	}
	if strings.Contains(text, "b1") || out.Conclusion().Warned {
		t.Fatalf("items leaked or warned:\n%s", text)
	}
}

func TestC13_002_LoneSkippedItemFoldsIntoItsGroupTally(t *testing.T) {
	out, buf := harness.New(t)
	g := out.Group("branches")
	own := g.Task("branches")
	own.Summary("2 checked")
	own.Define(func(context.Context) error { return nil })
	skipTask(g, "keep", "protected")
	_ = g.Wait()
	text := harness.Text(out, buf)
	if !strings.Contains(text, "✓ branches  2 checked") || !strings.Contains(text, "- skipped 1 (protected)") {
		t.Fatalf("lone skipped item:\n%s", text)
	}
}

func TestC13_003_GroupWithoutSummaryHasNoHeaderAndSequenceKeepsOne(t *testing.T) {
	out, buf := harness.New(t)
	g := out.Group("grp")
	_ = harness.Succeed(g.Task("a"))
	s := out.Sequence("seq")
	_ = harness.Succeed(s.Task("b"))
	text := harness.Text(out, buf)
	if strings.Contains(text, "grp") || !strings.Contains(text, "seq") {
		t.Fatalf("headers:\n%s", text)
	}
}

func TestC13_004_DuplicateNamedHeaderlessChildUsesContainerPath(t *testing.T) {
	out, buf := harness.New(t)
	_ = harness.Succeed(out.Task("build"))
	_ = harness.Succeed(out.Group("g").Task("build"))
	if text := harness.Text(out, buf); !strings.Contains(text, "g › build") {
		t.Fatalf("container path missing:\n%s", text)
	}
}

func TestC13_005_FinishedNoOpChildIsHiddenBesideVisibleContent(t *testing.T) {
	out, buf := harness.New(t)
	g := out.Group("g")
	noop := g.Task("idle check")
	noop.Verify(func(context.Context) (bool, error) { return true, nil })
	noop.Define(func(context.Context) error { return nil })
	work := g.Task("real work")
	work.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "file", Quantity: 1},
			func(context.Context) error { return nil })
	})
	_ = g.Wait()
	if text := harness.Text(out, buf); strings.Contains(text, "idle check") || !strings.Contains(text, "real work") {
		t.Fatalf("no-op visibility:\n%s", text)
	}
}

func TestC13_006_HumanShowsFiveProblemsThenCountMachineKeepsAll(t *testing.T) {
	out, buf := harness.New(t)
	task := out.Task("lint")
	task.Define(func(context.Context) error {
		for i := range 8 {
			task.Problem(fmt.Sprintf("violation-%d", i))
		}
		return nil
	})
	_ = task.Wait()
	text := harness.Text(out, buf)
	if !strings.Contains(text, "and 3 more failures") || strings.Contains(text, "violation-7") {
		t.Fatalf("human cap:\n%s", text)
	}
	if got := len(harness.MustFind(t, out, "lint").Problems); got != 8 {
		t.Fatalf("snapshot keeps %d problems, want 8", got)
	}
}

func TestC13_007_JSONKeepsWhatHumanOutputHides(t *testing.T) {
	out, _ := harness.New(t)
	g := out.Group("g")
	idle := g.Task("idle check")
	idle.Verify(func(context.Context) (bool, error) { return true, nil })
	idle.Define(func(context.Context) error { return nil })
	skipTask(g, "skipped one", "protected")
	_ = g.Wait()
	doc := harness.RunDocument(t, out)
	names := map[string]bool{}
	var dispositions []map[string]any
	for _, task := range harness.Objects(t, harness.Object(t, doc, "data"), "tasks") {
		names[fmt.Sprint(task["name"])] = true
		if d, ok := task["dispositions"].([]any); ok {
			for _, item := range d {
				dispositions = append(dispositions, item.(map[string]any))
			}
		}
	}
	if !names["idle check"] || !names["skipped one"] || len(dispositions) != 1 || dispositions[0]["reason"] != "protected" {
		t.Fatalf("tasks=%v dispositions=%v", names, dispositions)
	}
}

func TestC13_008_WarningRendersItsOnSubjectOnEveryRow(t *testing.T) {
	out, buf := harness.New(t)
	task := out.Task("check jobs")
	task.Define(func(context.Context) error {
		for _, subject := range []string{"job-a", "job-b"} {
			task.Problem("slow", evo.Severity(evo.SeverityWarning), evo.On(subject))
		}
		return nil
	})
	_ = task.Wait()
	text := harness.Text(out, buf)
	if !strings.Contains(text, "job-a") || !strings.Contains(text, "job-b") {
		t.Fatalf("On subjects missing:\n%s", text)
	}
}

func TestC13_009_GlyphsHaveOneMeaningEach(t *testing.T) {
	out, buf := harness.New(t, func(c *evo.Config) { c.Glyphs = evo.GlyphsUnicode })
	_ = harness.Succeed(out.Task("ok"))
	bad := out.Task("bad")
	bad.Define(func(context.Context) error { bad.Fail("broke"); return nil })
	_ = bad.Wait()
	blocked := out.Task("held")
	blocked.Define(func(context.Context) error { blocked.Block("refused"); return nil })
	_ = blocked.Wait()
	warn := out.Task("warned")
	warn.Define(func(context.Context) error {
		warn.Problem("drift", evo.Severity(evo.SeverityWarning))
		return nil
	})
	_ = warn.Wait()
	_ = out.Task("later")
	text := harness.Text(out, buf)
	for _, want := range []string{"✓ ok", "✗ bad", "⊘ held", "! ", "- later"} {
		if !strings.Contains(text, want) {
			t.Errorf("glyph row %q missing:\n%s", want, text)
		}
	}
}
