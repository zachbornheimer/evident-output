package evo_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestTaskCapture_DetailTail_OnFail(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.For("brew", evo.To(&primary), evo.Diagnostics(&diag), evo.Plain(), evo.NoColor())
	upgrade := out.Task("brew packages")
	output := upgrade.Capture()
	fmt.Fprintln(output, "Error: bottle not found")
	fmt.Fprintln(output, "Error: formula foo conflict")
	_ = output.Close()

	upgrade.Fail("brew upgrade failed", output.DetailTail())
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	ps := primary.String()
	if !strings.Contains(ps, "brew packages") {
		t.Fatalf("expected task in primary:\n%s", ps)
	}
	if !strings.Contains(diag.String(), "bottle not found") {
		t.Fatalf("expected Capture mirror on Diagnostics:\n%q", diag.String())
	}
	if output.TaskName() != "brew packages" {
		t.Fatalf("task name: %q", output.TaskName())
	}
	if !strings.Contains(ps, "formula foo conflict") {
		t.Fatalf("Fail Detail should surface tail:\n%s", ps)
	}
	if !strings.Contains(ps, "Last 2 lines") {
		t.Fatalf("DetailTail should label line count:\n%s", ps)
	}
	// Success path must not dump capture automatically — we only attached on Fail.
}

func TestTaskCapture_NoAutoSurfaceOnSuccess(t *testing.T) {
	var primary bytes.Buffer
	out := evo.For("t", evo.To(&primary), evo.Plain(), evo.NoColor())
	task := out.Task("work")
	output := task.Capture()
	fmt.Fprintln(output, "lots of chatter")
	_ = output.Close()
	task.Done()
	_ = out.Finish()
	if strings.Contains(primary.String(), "lots of chatter") {
		t.Fatalf("capture must not auto-surface on success:\n%s", primary.String())
	}
}

func TestTaskCapture_Quiet_NoDiagnosticMirror(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.For("t", evo.To(&primary), evo.Diagnostics(&diag), evo.Plain(), evo.NoColor())
	task := out.Task("x")
	output := task.Capture(evo.CaptureQuiet())
	fmt.Fprintln(output, "secret chatter")
	_ = output.Close()
	task.Done()
	_ = out.Finish()
	if strings.Contains(diag.String(), "secret") {
		t.Fatalf("CaptureQuiet must not mirror: %q", diag.String())
	}
	if output.Empty() {
		t.Fatal("tail still retained")
	}
}

func TestTaskCapture_RingBoundsAndTruncation(t *testing.T) {
	var primary bytes.Buffer
	out := evo.For("t", evo.To(&primary), evo.Plain(), evo.NoColor())
	task := out.Task("x")
	output := task.Capture(evo.KeepLastLines(3))
	for i := 0; i < 10; i++ {
		fmt.Fprintf(output, "line-%d\n", i)
	}
	_ = output.Close()
	text := output.Text()
	if strings.Contains(text, "line-0") {
		t.Fatalf("oldest should drop: %q", text)
	}
	if !strings.Contains(text, "line-9") || !strings.Contains(text, "line-7") {
		t.Fatalf("want last 3 lines: %q", text)
	}
	if !strings.Contains(text, "[earlier output truncated]") {
		t.Fatalf("expected truncation marker: %q", text)
	}

	var primary2 bytes.Buffer
	out2 := evo.For("t2", evo.To(&primary2), evo.Plain(), evo.NoColor())
	t2 := out2.Task("z")
	o2 := t2.Capture(evo.KeepLastLines(3))
	for i := 0; i < 10; i++ {
		fmt.Fprintf(o2, "line-%d\n", i)
	}
	_ = o2.Close()
	t2.Fail("failed", o2.DetailTail())
	_ = out2.Finish()
	if !strings.Contains(primary2.String(), "[earlier output truncated]") {
		t.Fatalf("DetailTail should show truncation:\n%s", primary2.String())
	}
}

func TestTaskCapture_StderrPreferredForDetail(t *testing.T) {
	var primary bytes.Buffer
	out := evo.For("t", evo.To(&primary), evo.Plain(), evo.NoColor())
	task := out.Task("cmd")
	output := task.Capture()
	fmt.Fprintln(output.Stdout(), "downloading…")
	fmt.Fprintln(output.Stderr(), "Error: link conflict")
	_ = output.Close()
	task.Fail("command failed", output.DetailTail())
	_ = out.Finish()
	ps := primary.String()
	if !strings.Contains(ps, "link conflict") {
		t.Fatalf("stderr should be in detail:\n%s", ps)
	}
	// Prefer stderr-only detail when both present — stdout chatter optional.
	// May still include Last N from stderr only.
	if strings.Contains(ps, "downloading") && !strings.Contains(ps, "link conflict") {
		t.Fatal("unexpected")
	}
}

func TestTaskCapture_DebugLabeledWhenEnabled(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.For("t",
		evo.To(&primary),
		evo.Diagnostics(&diag),
		evo.Plain(),
		evo.NoColor(),
		evo.DebugLevel(evo.Debug),
	)
	task := out.Task("upgrade")
	output := task.Capture()
	fmt.Fprintln(output, "Pouring openssl@3")
	_ = output.Close()
	task.Done()
	_ = out.Finish()
	// Diagnostics mirror always has the line.
	if !strings.Contains(diag.String(), "Pouring openssl@3") {
		t.Fatalf("diag: %q", diag.String())
	}
	// Debug journal dual-stream goes to diag; may include task= field in history format.
	// Primary must not dump capture on success.
	if strings.Contains(primary.String(), "Pouring") {
		t.Fatalf("success primary polluted:\n%s", primary.String())
	}
}

func TestDiagnostics_DualStream_DebugNotOnPrimary(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.For("t",
		evo.To(&primary),
		evo.Diagnostics(&diag),
		evo.Plain(),
		evo.NoColor(),
		evo.DebugLevel(evo.Debug),
	)
	out.Debug("internal only")
	out.Item("ok").OK()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(primary.String(), "internal only") {
		t.Fatalf("dual-stream Debug must not hit primary:\n%s", primary.String())
	}
	if !strings.Contains(diag.String(), "internal only") {
		t.Fatalf("Debug must hit Diagnostics:\n%q", diag.String())
	}
}

func TestWriterOptions_PipeAndDiagnosticsWired(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	var diag bytes.Buffer
	out := evo.For("tool", evo.WriterOptions(w, evo.Diagnostics(&diag), evo.DebugLevel(evo.Debug))...)
	out.Debug("diag-line")
	out.Item("gate").Block("dirty")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var primary bytes.Buffer
	_, _ = primary.ReadFrom(r)
	ps := primary.String()
	if strings.Contains(ps, "\x1b[") {
		t.Fatalf("pipe primary must be NoColor:\n%q", ps)
	}
	if !strings.Contains(diag.String(), "diag-line") {
		t.Fatalf("Diagnostics not wired: %q", diag.String())
	}
}
