package lifecycle_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func quiet() evo.Config {
	return evo.Config{Plain: true, Stdout: io.Discard, Stderr: io.Discard}
}

func TestC02_030_InitInstallsDefaultUnlessIsolated(t *testing.T) {
	prev := evo.Default()
	t.Cleanup(func() { evo.SetDefault(prev) })
	isolated := quiet()
	isolated.Isolated = true
	iso := evo.Init(isolated)
	t.Cleanup(func() { _ = iso.Close() })
	if evo.Default() != prev {
		t.Fatal("an Isolated Init replaced the default instance")
	}
	inst := evo.Init(quiet())
	t.Cleanup(func() { _ = inst.Close() })
	if evo.Default() != inst {
		t.Fatal("a non-isolated Init did not install the default instance")
	}
}

func TestC02_031_MainReturnsTheExitCodeWithoutExiting(t *testing.T) {
	prev := evo.Default()
	t.Cleanup(func() { evo.SetDefault(prev) })
	evo.Init(quiet())
	if code := evo.Main(func(context.Context) error { return errors.New("boom") }); code != evo.ExitFailed {
		t.Fatalf("Main = %d, want %d", code, evo.ExitFailed)
	}
}

func TestC02_032_RunResultCarriesConclusionAndError(t *testing.T) {
	out, _ := harness.New(t)
	boom := errors.New("boom")
	res := out.Run(context.Background(), func(context.Context) error { return boom })
	if !errors.Is(res.Err, boom) || res.ExitCode() != evo.ExitFailed || res.Conclusion.State != evo.StateFailed {
		t.Fatalf("result = %+v", res)
	}
}

func TestC02_033_CloseIsIdempotentAndFinishesWhenNeeded(t *testing.T) {
	out, buf := harness.New(t)
	if err := harness.Succeed(out.Task("a")); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	concluded := buf.Len()
	if err := out.Close(); err != nil || buf.Len() != concluded {
		t.Fatalf("second Close: err=%v, wrote %d more bytes", err, buf.Len()-concluded)
	}
	if out.Conclusion().State == "" {
		t.Fatal("Close did not run Finish")
	}
}

func TestC02_034_DryRunAnnouncesHeaderAndNeverRunsMutations(t *testing.T) {
	out, buf := harness.New(t, func(c *evo.Config) { c.DryRun = true; c.Subject = "repo" })
	mutated := false
	task := out.Task("prune")
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "branch", Quantity: 2},
			func(context.Context) error { mutated = true; return nil })
	})
	_ = task.Wait()
	text := harness.Text(out, buf)
	if mutated {
		t.Fatal("a dry run executed the mutation callback")
	}
	if !strings.Contains(text, "[dry-run] repo") || !strings.Contains(text, "[planned] prune") {
		t.Fatalf("dry-run output:\n%s", text)
	}
}

func TestC02_035_PreviewPlansWithoutTheDryRunTag(t *testing.T) {
	out, buf := harness.New(t, func(c *evo.Config) { c.Preview = true; c.Subject = "repo" })
	task := out.Task("prune")
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "branch", Quantity: 2},
			func(context.Context) error { return nil })
	})
	_ = task.Wait()
	text := harness.Text(out, buf)
	if strings.Contains(text, "[dry-run]") || !strings.Contains(text, "[planned]") {
		t.Fatalf("preview output:\n%s", text)
	}
}

func TestC02_036_DefineNeverRunsTheCallbackInline(t *testing.T) {
	out, _ := harness.New(t)
	gate := make(chan struct{})
	task := out.Task("job")
	task.Define(func(context.Context) error { <-gate; return nil })
	close(gate)
	if err := task.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestC02_037_WaitReturnsTheAggregateError(t *testing.T) {
	out, _ := harness.New(t)
	boom := errors.New("boom")
	if err := out.Task("job").Define(func(context.Context) error { return boom }).Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait = %v", err)
	}
}

func TestC02_038_KeyFreezesAndRepeatingItIsIdempotent(t *testing.T) {
	out, _ := harness.New(t)
	task := out.Task("job")
	task.Key("job-1").Key("job-1")
	if err := harness.Succeed(task); err != nil {
		t.Fatal(err)
	}
	if got := harness.MustFind(t, out, "job").Key; got != "job-1" {
		t.Fatalf("key = %q", got)
	}
}

func TestC02_039_SiblingNamesAreUniquePerParent(t *testing.T) {
	out, _ := harness.New(t)
	first := out.Task("dup")
	out.Task("dup")
	_ = harness.Succeed(first)
	err := out.Finish()
	if !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("Finish = %v, want ErrDuplicateSiblingName", err)
	}
}
