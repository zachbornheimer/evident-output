package exitcodes_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func TestC17_001_ConfirmDeclineConcludesBlocked(t *testing.T) {
	out, _ := harness.New(t, func(c *evo.Config) { c.Stdin = strings.NewReader("n\n") })
	if out.Confirm("Delete the branches?") {
		t.Fatal("a declined Confirm returned true")
	}
	_ = out.Finish()
	if c := out.Conclusion(); c.State != evo.StateBlocked || c.ExitCode != evo.ExitBlocked {
		t.Fatalf("conclusion %v exit %d", c.State, c.ExitCode)
	}
}

func TestC17_002_FailedExitCodeOverridesExitTwo(t *testing.T) {
	const custom = 7
	out, _ := harness.New(t, func(c *evo.Config) { c.FailedExitCode = custom })
	_ = out.Task("bad").Define(func(context.Context) error { return errors.New("boom") }).Wait()
	_ = out.Finish()
	if got := out.Conclusion().ExitCode; got != custom {
		t.Fatalf("exit = %d, want %d", got, custom)
	}
}

func TestC17_003_PartialModifiesTheHeadlineNeverTheExitCode(t *testing.T) {
	out, _ := harness.New(t)
	_ = out.Task("never defined")
	_ = out.Finish()
	c := out.Conclusion()
	if !c.Partial || c.ExitCode != evo.ExitOK {
		t.Fatalf("partial=%v exit=%d", c.Partial, c.ExitCode)
	}
}

func TestC17_004_RunErrorIsRecordedOnlyWhenNothingFailed(t *testing.T) {
	bare, bareBuf := harness.New(t)
	res := bare.Run(context.Background(), func(context.Context) error { return errors.New("outer-only") })
	if res.ExitCode() != evo.ExitFailed || !strings.Contains(bareBuf.String(), "outer-only") {
		t.Fatalf("lone run error: exit %d\n%s", res.ExitCode(), bareBuf)
	}
	out, buf := harness.New(t)
	out.Run(context.Background(), func(context.Context) error {
		_ = out.Task("bad").Define(func(context.Context) error { return errors.New("inner-cause") }).Wait()
		return errors.New("outer-echo")
	})
	if text := buf.String(); !strings.Contains(text, "inner-cause") || strings.Contains(text, "outer-echo") {
		t.Fatalf("run error duplicated a task failure:\n%s", text)
	}
}
