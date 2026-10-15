package evo

import (
	"bytes"
	"strings"
	"testing"
)

// nonTTYConfig builds a Plain/NoColor Config that writes into buf.
// Buffers are never TTYs, so this is the flat/CI path zq pilots use.
func nonTTYConfig(title string, buf *bytes.Buffer) Config {
	return Config{Name: title, Output: Plain, Facades: Facades{Stdout: buf, Stderr: buf}}
}

// TestPlain_MultiLineDetailPreservedAsBlock is the P3 contract: multi-line
// Problem Detail (capture tails / diffs) must render as an indented multi-line
// block under the fail row, not a single joined line with newlines collapsed.
func TestPlain_MultiLineDetailPreservedAsBlock(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	// Multi-line detail matching a gofmt-style diff (the zq pilot shape).
	detail := "diff main.go.orig main.go\n--- main.go.orig\n+++ main.go\n@@ -1,3 +1,5 @@\n package main"
	task := out.Task("gofmt check")
	task.fail("gofmt check exited 1", Detail(detail))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Newlines must survive: each detail line is its own physical line.
	if !strings.Contains(got, "diff main.go.orig main.go\n") {
		t.Fatalf("P3: multi-line Detail flattened (missing newline after first detail line):\n%s", got)
	}
	if !strings.Contains(got, "--- main.go.orig\n") {
		t.Fatalf("P3: multi-line Detail flattened (missing --- line):\n%s", got)
	}
	if !strings.Contains(got, "+++ main.go\n") {
		t.Fatalf("P3: multi-line Detail flattened (missing +++ line):\n%s", got)
	}
	// Regression of the pre-fix bug shape: everything joined with spaces on one line.
	joinedBug := "diff main.go.orig main.go --- main.go.orig +++ main.go"
	// Strip ANSI just in case; ColorNever should already be clean.
	if strings.Contains(got, joinedBug) {
		t.Fatalf("P3: Detail collapsed onto one joined line (old bug shape):\n%s", got)
	}
}

// TestPlain_FailDetailDoesNotEchoSummary is the P4 contract: when Detail is
// present, the └─ block is the tail only — not "summary     detail".
func TestPlain_FailDetailDoesNotEchoSummary(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	summary := "gofmt check exited 1"
	detail := "diff main.go.orig main.go\n--- main.go.orig\n+++ main.go"
	out.Task("gofmt check").fail(summary, Detail(detail))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	// Fail summary still appears on the ✗ row.
	if !strings.Contains(got, "✗ gofmt check") {
		t.Fatalf("expected fail glyph row:\n%s", got)
	}
	if !strings.Contains(got, summary) {
		t.Fatalf("expected summary on fail row:\n%s", got)
	}
	// └─ block must not re-echo the summary as a header before the tail.
	// Old shape: "└─ gofmt check exited 1     Last …" or "└─ summary     detail".
	echoHeader := "└─ " + summary
	if strings.Contains(got, echoHeader) {
		t.Fatalf("P4: fail summary echoed as └─ header:\n%s", got)
	}
	// Detail body is present.
	if !strings.Contains(got, "diff main.go.orig main.go") {
		t.Fatalf("expected detail tail in output:\n%s", got)
	}
}

// TestPlain_StandaloneTaskBeforeTrailingPrintf is the P2 contract: standalone
// Tasks that resolve before a Printf summary appear before that summary in the
// primary stream (creation/completion order, not all-tasks-at-Finish).
//
// Contract: in plain/non-TTY mode, a terminal standalone Task streams on
// resolution (like Items). Printf that runs after Task.Fail therefore cannot
// appear above the task row.
func TestPlain_StandaloneTaskBeforeTrailingPrintf(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("zq", &buf))
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("gofmt check")
	task.fail("gofmt check exited 1", Detail("diff main.go.orig main.go\n--- a\n+++ b"))
	// After Fail, progressive plain path should already show the task row.
	mid := buf.String()
	if !strings.Contains(mid, "gofmt check") || !strings.Contains(mid, "✗") {
		t.Fatalf("P2: standalone Task must stream on Fail before Printf; mid=%q", mid)
	}

	out.printf("[SUMMARY] failed: gofmt check\n")
	out.printf("[SUMMARY] run `zq fix` to auto-fix formatting\n")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	taskIdx := strings.Index(got, "✗ gofmt check")
	summaryIdx := strings.Index(got, "[SUMMARY] failed:")
	if taskIdx < 0 {
		t.Fatalf("missing task fail row:\n%s", got)
	}
	if summaryIdx < 0 {
		t.Fatalf("missing summary line:\n%s", got)
	}
	if taskIdx > summaryIdx {
		t.Fatalf("P2: task row must appear before trailing Printf summary:\n%s", got)
	}
}

// TestPlain_MixedPrintfThenTaskStillDeterministic documents residual ordering
// when Printf happens before a late Task resolve: progressive task emission
// still places the task row after earlier lines, and residual does not reorder
// already-streamed content.
func TestPlain_MixedPrintfThenTaskStillDeterministic(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(nonTTYConfig("tool", &buf))
	t.Cleanup(func() { _ = out.close() })

	out.printf("starting checks\n")
	task := out.Task("shellcheck")
	// Task still pending; mid-run message.
	out.printf("still running\n")
	succeed(task)
	out.printf("all done\n")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// Order: starting → still running → ✓ shellcheck → all done → conclusion
	order := []string{"starting checks", "still running", "shellcheck", "all done"}
	prev := -1
	for _, s := range order {
		i := strings.Index(got, s)
		if i < 0 {
			t.Fatalf("missing %q in:\n%s", s, got)
		}
		if i < prev {
			t.Fatalf("order violation around %q:\n%s", s, got)
		}
		prev = i
	}
}
