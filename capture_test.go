package evo_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestCapture_TailForFailDetail_NoLivePollution(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.For("brew", evo.To(&primary), evo.Diagnostics(&diag), evo.Plain(), evo.NoColor())
	cap := out.Capture()
	fmt.Fprintln(cap, "Error: bottle not found")
	fmt.Fprintln(cap, "Error: formula foo conflict")
	_ = cap.Close()

	task := out.Task("brew upgrade")
	task.Fail("brew upgrade failed", evo.DetailTail(cap))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	ps := primary.String()
	if !strings.Contains(ps, "brew upgrade") {
		t.Fatalf("expected task in primary:\n%s", ps)
	}
	if !strings.Contains(diag.String(), "bottle not found") {
		t.Fatalf("expected Capture mirror on Diagnostics:\n%q", diag.String())
	}
	if !strings.Contains(cap.Tail(), "formula foo conflict") {
		t.Fatalf("tail: %q", cap.Tail())
	}
	if !strings.Contains(ps, "formula foo conflict") {
		t.Fatalf("Fail Detail should surface tail on primary problem:\n%s", ps)
	}
}

func TestCapture_Quiet_NoDiagnosticMirror(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.For("t", evo.To(&primary), evo.Diagnostics(&diag), evo.Plain(), evo.NoColor())
	cap := out.Capture(evo.CaptureQuiet())
	fmt.Fprintln(cap, "secret chatter")
	_ = cap.Close()
	out.Item("x").OK()
	_ = out.Finish()
	if strings.Contains(diag.String(), "secret") {
		t.Fatalf("CaptureQuiet must not mirror: %q", diag.String())
	}
	if cap.Empty() {
		t.Fatal("tail still retained")
	}
}

func TestCapture_RingBounds(t *testing.T) {
	var primary bytes.Buffer
	out := evo.For("t", evo.To(&primary), evo.Plain(), evo.NoColor())
	cap := out.Capture(evo.CaptureLines(3))
	for i := 0; i < 10; i++ {
		fmt.Fprintf(cap, "line-%d\n", i)
	}
	_ = cap.Close()
	tail := cap.Tail()
	if strings.Contains(tail, "line-0") {
		t.Fatalf("oldest should drop: %q", tail)
	}
	if !strings.Contains(tail, "line-9") || !strings.Contains(tail, "line-7") {
		t.Fatalf("want last 3 lines: %q", tail)
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
	if !strings.Contains(primary.String(), "ok") {
		t.Fatalf("human item missing:\n%s", primary.String())
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
	if !strings.Contains(ps, "gate") || !strings.Contains(ps, "dirty") {
		t.Fatalf("human content:\n%s", ps)
	}
	if !strings.Contains(diag.String(), "diag-line") {
		t.Fatalf("Diagnostics not wired via WriterOptions: %q", diag.String())
	}
	if strings.Contains(ps, "diag-line") {
		t.Fatalf("debug leaked to primary under dual stream:\n%s", ps)
	}
}
