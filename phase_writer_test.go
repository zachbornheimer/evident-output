package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestPhaseWriter_SplitsOnLF_AcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "t", Stdout: &buf, Stderr: &buf})
	task := out.Task("push")
	w := task.PhaseWriter()

	// A line split across two Write calls must still become one Phase update.
	if _, err := w.Write([]byte("clon")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("ing repo\n")); err != nil {
		t.Fatal(err)
	}
	if got := task.Snapshot().Phase; got != "cloning repo" {
		t.Fatalf("phase after split LF write = %q, want %q", got, "cloning repo")
	}

	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseWriter_CRDelimitsProgressFrames(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "t", Stdout: &buf, Stderr: &buf})
	task := out.Task("download")
	w := task.PhaseWriter()

	// A \r-driven progress bar (no LF between frames) must update Phase per frame.
	if _, err := w.Write([]byte("50%\r75%\r100%\n")); err != nil {
		t.Fatal(err)
	}
	if got := task.Snapshot().Phase; got != "100%" {
		t.Fatalf("phase after CR-delimited frames = %q, want %q", got, "100%")
	}

	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseWriter_BlankLinesIgnored(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "t", Stdout: &buf, Stderr: &buf})
	task := out.Task("sync")
	w := task.PhaseWriter()

	if _, err := w.Write([]byte("first\n\n   \nsecond\n")); err != nil {
		t.Fatal(err)
	}
	if got := task.Snapshot().Phase; got != "second" {
		t.Fatalf("phase = %q, want %q (blank lines must not clear it)", got, "second")
	}

	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseWriter_BytesLandInCapture_DetailTailAfterFail(t *testing.T) {
	var primary bytes.Buffer
	out := evo.Init(evo.Config{Title: "t", Stdout: &primary, Stderr: &primary})
	task := out.Task("push")
	w := task.PhaseWriter()

	if _, err := w.Write([]byte("pushing feat/a\nremote: rejected non-fast-forward\n")); err != nil {
		t.Fatal(err)
	}

	// The task's Capture ring (get-or-create, same instance PhaseWriter fed)
	// must carry the child output as failure evidence.
	task.Fail("push failed", task.Evidence().DetailTail())
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(primary.String(), "remote: rejected non-fast-forward") {
		t.Fatalf("DetailTail missing PhaseWriter evidence:\n%s", primary.String())
	}
}

// TestPhaseWriter_UnboundedPendingFragmentIsCapped is the red-first case for
// phase_writer.go's unbounded buffer: a child that emits far more than one
// screen's worth of bytes with no line terminator must not grow the
// pending-fragment buffer without limit — once it exceeds the cap, the
// fragment flushes as a phase line on its own.
func TestPhaseWriter_UnboundedPendingFragmentIsCapped(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "t", Stdout: &buf, Stderr: &buf})
	task := out.Task("build")
	w := task.PhaseWriter()

	// One line-terminator-free write far larger than any reasonable phase
	// text — no \r or \n anywhere, so without a cap this buffers forever.
	const oversized = 8 * 1024 // 2x the 4 KiB cap
	payload := make([]byte, oversized)
	for i := range payload {
		payload[i] = 'x'
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}

	if got := task.Snapshot().Phase; len(got) == 0 {
		t.Fatal("oversized line-less fragment never flushed as a phase line")
	} else if len(got) > oversized {
		t.Fatalf("phase text longer than the input payload: %d bytes", len(got))
	}

	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseWriter_SanitizesHostileLines(t *testing.T) {
	var primary bytes.Buffer
	out := evo.Init(evo.Config{Title: "t", Stdout: &primary, Stderr: &primary})
	task := out.Task("push")
	w := task.PhaseWriter()

	const payload = "\x1b[31mFAKE OK\x1b[0m pushing feat/a\n"
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}

	if got := task.Snapshot().Phase; strings.Contains(got, "\x1b") {
		t.Fatalf("Phase leaked raw ESC: %q", got)
	}

	task.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(primary.String(), "\x1b[31m") {
		t.Fatalf("rendered output leaked raw CSI:\n%s", primary.String())
	}
}
