package evo

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestFail_AutoAttachesDetailTail_WhenEvidenceNonEmptyAndNoExplicitDetail is
// beginner-2: a Fail/Block call with a non-empty evidence ring and no
// explicit Detail auto-attaches DetailTail — the evidence a caller already
// gathered via Evidence() is exactly the detail a Fail row needs, so
// DetailTail is no longer an opt-in step a caller has to remember.
func TestFail_AutoAttachesDetailTail_WhenEvidenceNonEmptyAndNoExplicitDetail(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	task := out.Task("build")
	output := task.capture()
	_, _ = fmt.Fprintln(output, "error: undefined symbol foo")
	task.fail("compile failed")

	_ = out.finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "undefined symbol foo") {
		t.Fatalf("Fail did not auto-attach the evidence tail, got:\n%s", rendered)
	}
}

// TestBlock_AutoAttachesDetailTail mirrors the Fail case for Block.
func TestBlock_AutoAttachesDetailTail(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	task := out.Task("policy check")
	output := task.capture()
	_, _ = fmt.Fprintln(output, "policy violation: missing signature")
	task.block("policy check failed")

	_ = out.finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "missing signature") {
		t.Fatalf("Block did not auto-attach the evidence tail, got:\n%s", rendered)
	}
}

// TestFail_ExplicitDetail_NotOverwrittenByEvidence proves an explicit Detail
// still wins over the evidence ring — auto-attach only fills a gap, it never
// clobbers a caller's own wording.
func TestFail_ExplicitDetail_NotOverwrittenByEvidence(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	task := out.Task("build")
	output := task.capture()
	_, _ = fmt.Fprintln(output, "raw evidence noise")
	task.fail("compile failed", Detail("caller-chosen detail"))

	_ = out.finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "caller-chosen detail") {
		t.Fatalf("explicit Detail missing, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "raw evidence noise") {
		t.Fatalf("explicit Detail should not be overwritten by evidence, got:\n%s", rendered)
	}
}
