package evo

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

const (
	sharedTerminalPayloadLines = 5
	sharedTerminalRunningRows  = 3
	runningRowDetail           = "running"
)

// Stdout and the live region's stderr are one terminal when a program runs in a shell with nothing
// redirected. A payload written while the region is on screen must reach the final screen whole and in
// order: not glued to a live row, not painted over by the next repaint, and with no stale live row left
// behind.
func TestResultWriterPayloadSurvivesSharedTerminal(t *testing.T) {
	pty := newPtyScreen(t)
	out := initRun(Config{Facades: Facades{Stdout: pty.Stdout, Stderr: pty.Stderr}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	var work []*Task
	for i := range sharedTerminalRunningRows {
		task := out.Task(fmt.Sprintf("work %d", i))
		task.Doing(runningRowDetail)
		work = append(work, task)
	}
	out.forceLiveVisibleForTest()
	payload := out.ResultWriter()
	var want []string
	for i := 1; i <= sharedTerminalPayloadLines; i++ {
		line := fmt.Sprintf("PAYLOAD-LINE-%d", i)
		want = append(want, line)
		if _, err := fmt.Fprintln(payload, line); err != nil {
			t.Fatalf("write payload line %d: %v", i, err)
		}
		out.forceLiveVisibleForTest()
	}
	for _, task := range work {
		succeed(task)
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	_ = out.close()

	screen := pty.Lines(t)
	var got []string
	for _, row := range screen {
		if strings.Contains(row, "PAYLOAD") {
			got = append(got, row)
		}
		if strings.Contains(row, runningRowDetail) {
			t.Fatalf("stale live row %q left on the final screen:\n%s", row, strings.Join(screen, "\n"))
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("payload rows = %q, want %q; final screen:\n%s", got, want, strings.Join(screen, "\n"))
	}
}
