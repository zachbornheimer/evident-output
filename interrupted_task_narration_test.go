package evo

import (
	"bytes"
	"strings"
	"testing"
)

// Cancellation resolves a row underneath whoever was narrating it. A worker
// that was already inside its per-item step still reports that item, and the
// caller had no way to prevent it — the interrupt happened between their
// check and their call. Charging that as "resolve each task once" put a
// misuse warning at the top of every interrupted ledger, blaming the caller
// for the interrupt's own timing.
//
// A row the CALLER resolved is different: narrating that one is a real
// mistake and still reports.
func TestDoing_AfterCancellationIsSilentNotMisuse(t *testing.T) {
	transcript := narrateAfterResolve(t, func(task *Task) {
		task.cancel("interrupted")
	})
	if strings.Contains(transcript, "resolve each task once") {
		t.Fatalf("narrating a cancelled row blamed the caller:\n%s", transcript)
	}
}

func TestDoing_AfterCallerResolvedIsStillMisuse(t *testing.T) {
	transcript := narrateAfterResolve(t, func(task *Task) {
		succeed(task)
	})
	if !strings.Contains(transcript, "resolve each task once") {
		t.Fatalf("narrating a row the caller resolved must still report:\n%s", transcript)
	}
}

func narrateAfterResolve(t *testing.T, resolve func(*Task)) string {
	t.Helper()
	var buf bytes.Buffer
	out := initRun(Config{title: "zq", plain: true, stdout: &buf, stderr: &buf})
	task := out.Group("worktrees").Task("classify")
	task.Doing("scanning")
	resolve(task)

	// The straggler: a worker already inside its step reports one more item.
	task.Progress(27, 111)
	task.Doing("/Users/zb/Developer/Zysys/.worktrees/flight-land-docker")

	_ = out.finish()
	_ = out.close()
	return buf.String()
}
