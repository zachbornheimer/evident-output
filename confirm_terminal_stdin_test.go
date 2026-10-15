//go:build darwin || linux

package evo

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// typedTerminal is a real character device a person has typed answer onto: the slave end of a pty, so the
// Stdin facade is a terminal in the sense the OS reports, with no test-only marker.
func typedTerminal(t *testing.T, answer string) *os.File {
	t.Helper()
	master, slavePath, err := openPtyPair()
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	slave, err := os.OpenFile(slavePath, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		t.Fatalf("open pty slave: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close(); _ = master.Close() })
	if _, err := master.WriteString(answer); err != nil {
		t.Fatalf("type %q onto the pty: %v", answer, err)
	}
	return slave
}

// TestConfirmAsksWhenStdinIsATerminalInEveryOutput pins CONTRACT section 5: whether Confirm asks depends on
// stdin being a terminal, never on Output. Plain, JSON and JSONL ask a person at a terminal as Human does.
func TestConfirmAsksWhenStdinIsATerminalInEveryOutput(t *testing.T) {
	for name, output := range map[string]Output{"Human": Human, "Plain": Plain, "JSON": JSON, "JSONL": JSONL} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Output: output, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Stdin: typedTerminal(t, "y\n")}}
			out := initRun(cfg, tuning{color: colorNever})

			if !out.Confirm("proceed?") {
				t.Fatalf("Confirm(Output %s) with a terminal on stdin answering y = false, want true: it did not ask", name)
			}
			if got := out.snapshot().Tasks[0].State; got != succeeded {
				t.Fatalf("gate state = %v, want Succeeded", got)
			}
		})
	}
}

// TestConfirmWithoutATerminalRefusesWithNoTerminalToAnswer pins the other half of CONTRACT section 5: when
// stdin is not a terminal nobody can answer, the refusal says so in those words ("blocked by policy" is
// Exclude's word, not Confirm's), and stdin is never read.
func TestConfirmWithoutATerminalRefusesWithNoTerminalToAnswer(t *testing.T) {
	for name, output := range map[string]Output{"Human": Human, "Plain": Plain, "JSON": JSON, "JSONL": JSONL} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Output: output, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Stdin: bytes.NewBufferString("y\n")}}
			out := initRun(cfg, tuning{color: colorNever})

			if out.Confirm("proceed?") {
				t.Fatalf("Confirm(Output %s) with no terminal on stdin = true, want false", name)
			}
			item := out.snapshot().Tasks[0]
			if item.State != refused {
				t.Fatalf("gate state = %v, want Refused", item.State)
			}
			if len(item.Problems) == 0 || !strings.Contains(item.Problems[0].Summary, "no terminal to answer") {
				t.Fatalf("problems = %+v, want a summary naming %q", item.Problems, "no terminal to answer")
			}
		})
	}
}

// TestConfirmTreatsANullDeviceAsNoTerminal pins CONTRACT section 5 against the CI shape: stdin redirected from
// /dev/null is a character device but not a terminal, so nobody can answer and the gate refuses at once with
// the no-terminal wording, never prompting and never reporting a closed stdin.
func TestConfirmTreatsANullDeviceAsNoTerminal(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = null.Close() })
	var stderr bytes.Buffer
	cfg := Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &stderr, Stdin: null}}
	out := initRun(cfg, tuning{color: colorNever})

	if out.Confirm("proceed?") {
		t.Fatal("Confirm with /dev/null on stdin = true, want false")
	}
	item := out.snapshot().Tasks[0]
	if len(item.Problems) == 0 || item.Problems[0].Summary != "no terminal to answer" {
		t.Fatalf("problems = %+v, want summary %q", item.Problems, "no terminal to answer")
	}
	if strings.Contains(stderr.String(), "[y/N]") {
		t.Fatalf("Confirm prompted with no one to answer:\n%s", stderr.String())
	}
}
