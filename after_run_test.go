package evo_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func newAfterRunOutput(buf *bytes.Buffer) *evo.Output {
	return evo.Init(evo.Config{Isolated: true, Stdout: buf, Plain: true})
}

func TestAfterRun_RendersAfterTheFooterWithTheResult(t *testing.T) {
	var buf bytes.Buffer
	out := newAfterRunOutput(&buf)
	out.AfterRun(func(w io.Writer, r evo.Result) {
		_, _ = io.WriteString(w, "epilogue exit="+string(rune('0'+r.Conclusion.ExitCode))+"\n")
	})
	res := out.Run(context.Background(), func(ctx context.Context) error {
		out.Task("work").Define(func(context.Context) error { return errors.New("boom") })
		return nil
	})
	rendered := buf.String()
	idx := strings.Index(rendered, "epilogue exit=2")
	if idx < 0 || res.ExitCode() != 2 {
		t.Fatalf("epilogue missing or exit = %d:\n%s", res.ExitCode(), rendered)
	}
	if !strings.HasSuffix(rendered, "epilogue exit=2\n") || strings.Index(rendered, "boom") > idx {
		t.Fatalf("epilogue must be the last thing rendered, after everything Finish wrote:\n%s", rendered)
	}
}

func TestAfterRun_JSONFormatKeepsMachineStreamPure(t *testing.T) {
	var machine, human bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &machine, Stderr: &human, Plain: true, Format: evo.FormatJSON})
	out.AfterRun(func(w io.Writer, _ evo.Result) { _, _ = io.WriteString(w, "epilogue\n") })
	_ = out.Run(context.Background(), func(context.Context) error { return nil })
	if strings.Contains(machine.String(), "epilogue") {
		t.Fatalf("epilogue leaked into the machine stream:\n%s", machine.String())
	}
	if !strings.Contains(human.String(), "epilogue") {
		t.Fatalf("epilogue missing from the human stream:\n%s", human.String())
	}
}

func TestPrintAfterRunWithoutHookIsReportedMisuse(t *testing.T) {
	var buf bytes.Buffer
	out := newAfterRunOutput(&buf)
	_ = out.Run(context.Background(), func(context.Context) error { return nil })
	out.Println("too late")
	rendered := buf.String()
	if strings.Contains(rendered, "too late") {
		t.Fatalf("late text must not render:\n%s", rendered)
	}
	if !strings.Contains(rendered, "AfterRun") {
		t.Fatalf("late write must be reported and name AfterRun:\n%s", rendered)
	}
}
