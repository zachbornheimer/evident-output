// Package summary_test binds contract §30 "Task Summary" rules to the
// public evo API.
package summary_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const multiLineInput = "first line\nsecond line"

func newPlainOutput(buf *bytes.Buffer) *evo.Output {
	return evo.Init(evo.Config{Isolated: true, Plain: true, Color: evo.ColorNever, Stdout: buf, Stderr: buf})
}

func TestC30_011_SummaryIsOneSanitizedLineLastCallWinsEmptyClears(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	task.Summary(multiLineInput)
	if got := task.Snapshot().Summary; strings.ContainsAny(got, "\n\r") || got == "" {
		t.Fatalf("Summary(%q) snapshots %q, want one non-empty line", multiLineInput, got)
	}
	task.Summary("last call")
	if got := task.Snapshot().Summary; got != "last call" {
		t.Fatalf("after a second call Summary = %q, want %q", got, "last call")
	}
	task.Summary("")
	if got := task.Snapshot().Summary; got != "" {
		t.Fatalf("Summary(\"\") left %q, want it cleared", got)
	}
}

func TestC30_014_ContainerSummariesShareTheTaskSanitization(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf)
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("task")
	task.Summary(multiLineInput)
	want := task.Snapshot().Summary

	group := out.Group("group")
	group.Task("member").Define(func(context.Context) error { return nil })
	group.Summary(multiLineInput)
	_ = group.Wait()
	if got := group.Snapshot().Summary; got != want {
		t.Fatalf("GroupHandle.Summary snapshots %q, want the Task sanitization %q", got, want)
	}
	sequence := out.Sequence("sequence")
	sequence.Task("step").Define(func(context.Context) error { return nil })
	sequence.Summary(multiLineInput)
	_ = sequence.Wait()
	if got := sequence.Snapshot().Summary; got != want {
		t.Fatalf("SequenceHandle.Summary snapshots %q, want the Task sanitization %q", got, want)
	}
}
