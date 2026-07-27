package evo_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestCaptureSuccessIsSilentByDefault(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.New(evo.Config{Title: "brew", Stdout: &primary, Stderr: &diag})
	task := out.Task("brew")
	output := task.Capture()
	fmt.Fprintln(output, "Downloading bottle...")
	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(primary.String(), "Downloading") {
		t.Fatalf("success primary polluted:\n%s", primary.String())
	}
	if strings.Contains(diag.String(), "Downloading") {
		t.Fatalf("default Capture must not mirror to Diagnostics:\n%s", diag.String())
	}
	if output.Empty() {
		t.Fatal("evidence must still be retained")
	}
	if !strings.Contains(output.Text(), "Downloading bottle...") {
		t.Fatalf("retained: %q", output.Text())
	}
}

func TestTaskCapture_DetailTail_OnFail(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.New(evo.Config{Title: "brew", Stdout: &primary, Stderr: &diag})
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
	if !strings.Contains(ps, "formula foo conflict") {
		t.Fatalf("Fail Detail should surface tail:\n%s", ps)
	}
	// Still silent on diagnostics unless MirrorToDiagnostics.
	if strings.Contains(diag.String(), "bottle not found") {
		t.Fatalf("must not auto-mirror: %q", diag.String())
	}
}

func TestCapture_MirrorToDiagnostics_OptIn(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.New(evo.Config{Title: "t", Stdout: &primary, Stderr: &diag})
	task := out.Task("x")
	output := task.Capture(evo.MirrorToDiagnostics())
	fmt.Fprintln(output, "chatter")
	_ = output.Close()
	task.Done()
	_ = out.Finish()
	if !strings.Contains(diag.String(), "chatter") {
		t.Fatalf("opt-in mirror missing: %q", diag.String())
	}
}

func TestCaptureSeparateStreamsDoNotMergePartialLines(t *testing.T) {
	var primary bytes.Buffer
	out := evo.New(evo.Config{Title: "t", Stdout: &primary, Stderr: &primary})
	task := out.Task("cmd")
	output := task.Capture()
	io.WriteString(output.Stdout(), "download")
	io.WriteString(output.Stderr(), " failed\n")
	io.WriteString(output.Stdout(), " complete\n")
	_ = output.Close()

	// Must not synthesize "download failed" as one line.
	text := output.Text()
	if strings.Contains(text, "download failed") {
		t.Fatalf("partial lines merged across streams: %q", text)
	}
	if !strings.Contains(text, " failed") && !strings.Contains(text, "failed") {
		// stderr completed as " failed"
		t.Fatalf("stderr line missing: %q", text)
	}
	if !strings.Contains(text, "download complete") && !strings.Contains(text, " complete") {
		t.Fatalf("stdout line missing: %q", text)
	}
}

func TestCapture_RingBoundsAndTruncation(t *testing.T) {
	var primary bytes.Buffer
	out := evo.New(evo.Config{Title: "t", Stdout: &primary, Stderr: &primary})
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
	if !strings.Contains(text, "[earlier output truncated]") {
		t.Fatalf("expected truncation marker: %q", text)
	}
}

func TestMainRunErrorCannotRenderReady(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "tool", Stdout: &buf, Stderr: &buf})
	code := evo.Main(out, func(o *evo.Output) error {
		return fmt.Errorf("database unavailable")
	})
	if code != evo.ExitFailed {
		t.Fatalf("exit %d, want %d", code, evo.ExitFailed)
	}
	if strings.Contains(buf.String(), "[ready]") {
		t.Fatalf("must not render ready on run error:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "[failed]") && !strings.Contains(buf.String(), "command failed") {
		t.Fatalf("expected failure presentation:\n%s", buf.String())
	}
}

func TestMainRunErrorOutranksBlockedConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "tool", Stdout: &buf, Stderr: &buf})
	code := evo.Main(out, func(o *evo.Output) error {
		o.Item("policy").Block("not permitted")
		return fmt.Errorf("database connection failed")
	})
	if code != evo.ExitFailed {
		t.Fatalf("exit %d, want failed (not blocked-only): %d; out:\n%s", code, evo.ExitFailed, buf.String())
	}
	if !strings.Contains(buf.String(), "[failed]") {
		t.Fatalf("failed should outrank blocked:\n%s", buf.String())
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
	primary.ReadFrom(r)
	if !strings.Contains(diag.String(), "diag-line") {
		t.Fatalf("Diagnostics not wired: %q", diag.String())
	}
}

func TestDiagnostics_DualStream_DebugNotOnPrimary(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.NewWithOptions(
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
