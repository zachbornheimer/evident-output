package evo

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestFail_ExplicitDetailAndDetailTail_BothRender pins the precedence bug
// surfaced by the consumer re-syncs: an explicit Detail and an explicit
// output.DetailTail() passed to the same Fail/Block call used to silently
// overwrite one another (last ProblemOption applied wins) instead of both
// surviving. An explicit Detail is never silently discarded — it renders
// first, with the evidence tail as an additional evidence line underneath.
func TestFail_ExplicitDetailAndDetailTail_BothRender(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	task := out.Task("deploy")
	output := task.capture()
	_, _ = fmt.Fprintln(output, "raw evidence: connection refused")
	task.fail("deploy failed", Detail("friendly summary"), output.DetailTail())

	_ = out.finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "friendly summary") {
		t.Fatalf("explicit Detail missing, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "connection refused") {
		t.Fatalf("evidence tail missing, got:\n%s", rendered)
	}
	if strings.Index(rendered, "friendly summary") > strings.Index(rendered, "connection refused") {
		t.Fatalf("Detail must render before the evidence tail, got:\n%s", rendered)
	}
}

// TestFail_ExplicitDetailTailThenDetail_BothRender proves the fix is
// order-independent: DetailTail passed before Detail must not lose the
// evidence tail either, and Detail still renders first.
func TestFail_ExplicitDetailTailThenDetail_BothRender(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})

	task := out.Task("deploy")
	output := task.capture()
	_, _ = fmt.Fprintln(output, "raw evidence: connection refused")
	task.fail("deploy failed", output.DetailTail(), Detail("friendly summary"))

	_ = out.finish()

	rendered := buf.String()
	if !strings.Contains(rendered, "friendly summary") {
		t.Fatalf("explicit Detail missing, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "connection refused") {
		t.Fatalf("evidence tail missing, got:\n%s", rendered)
	}
	if strings.Index(rendered, "friendly summary") > strings.Index(rendered, "connection refused") {
		t.Fatalf("Detail must render before the evidence tail, got:\n%s", rendered)
	}
}
