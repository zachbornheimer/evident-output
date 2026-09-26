package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFail_AutoAttachesDetailTail_WhenCaptureNonEmptyAndNoExplicitDetail is
// beginner-2: a Fail/Block call with a non-empty capture ring and no
// explicit Detail auto-attaches DetailTail — the output a caller already
// gathered via Capture() is exactly the detail a Fail row needs, so
// DetailTail is no longer an opt-in step a caller has to remember.
func TestFail_AutoAttachesDetailTail_WhenCaptureNonEmptyAndNoExplicitDetail(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	task := out.Task("build")
	output := task.CaptureForTest()
	_, _ = fmt.Fprintln(output, "error: undefined symbol foo")
	task.Fail("compile failed")

	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "undefined symbol foo") {
		t.Fatalf("Fail did not auto-attach the capture tail, got:\n%s", rendered)
	}
}

// TestBlock_AutoAttachesDetailTail mirrors the Fail case for Block.
func TestBlock_AutoAttachesDetailTail(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	task := out.Task("policy check")
	output := task.CaptureForTest()
	_, _ = fmt.Fprintln(output, "policy violation: missing signature")
	task.Block("policy check failed")

	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "missing signature") {
		t.Fatalf("Block did not auto-attach the capture tail, got:\n%s", rendered)
	}
}

// TestFail_ExplicitDetail_NotOverwrittenByCapture proves an explicit Detail
// still wins over the capture ring — auto-attach only fills a gap, it never
// clobbers a caller's own wording.
func TestFail_ExplicitDetail_NotOverwrittenByCapture(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	task := out.Task("build")
	output := task.CaptureForTest()
	_, _ = fmt.Fprintln(output, "raw capture noise")
	task.Fail("compile failed", evo.Detail("caller-chosen detail"))

	_ = out.Finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "caller-chosen detail") {
		t.Fatalf("explicit Detail missing, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "raw capture noise") {
		t.Fatalf("explicit Detail should not be overwritten by capture, got:\n%s", rendered)
	}
}
