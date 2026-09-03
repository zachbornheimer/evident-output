package evo

import (
	"io"
	"os/exec"
	"path/filepath"
)

// Run executes cmd as this task's subprocess, wiring cmd.Stdout/cmd.Stderr
// through the same Capture + PhaseWriter plumbing PhaseWriter uses directly:
// each line becomes the task's live Phase, every byte is retained
// (redacted, bounded) in the task's Capture ring so DetailTail has evidence
// after Fail. If cmd.Stdout/cmd.Stderr already point somewhere (a caller
// wiring its own log file, say), Run tees into it rather than replacing it.
//
// If the task has no Phase yet, Run sets one from cmd's basename
// (filepath.Base(cmd.Path) or cmd.Args[0]) so a live view shows what's
// running before the child ever writes a line.
//
// Run does not touch cmd.Stdin and does not Suspend — a subprocess that
// needs the terminal (a prompt, a pager) stays on the explicit Suspend path.
// A context baked into cmd via exec.CommandContext still governs
// cancellation exactly as it would for a bare cmd.Run(); Run adds no
// context handling of its own.
//
// Run returns the subprocess error verbatim and never resolves the task —
// the caller chooses Done/Fail from the result:
//
//	cmd := exec.Command("go", "build", "./...")
//	if err := task.Run(cmd); err != nil {
//	    return task.Fail("build failed", Cause(err), task.Capture().DetailTail())
//	}
//	task.Done()
func (t *TaskHandle) Run(cmd *exec.Cmd) error {
	if t != nil && t.out != nil {
		t.ensurePhase(commandPhaseName(cmd))
		pw := &phaseWriter{task: t, capture: t.Capture()}
		cmd.Stdout = teeSubprocessWriter(cmd.Stdout, pw)
		cmd.Stderr = teeSubprocessWriter(cmd.Stderr, pw)
	}
	return cmd.Run()
}

// commandPhaseName is the human phase text for a not-yet-run *exec.Cmd: the
// basename of the resolved path when set, else the first argv element —
// exec.Command("go", "build") and exec.Command("/usr/bin/go", "build") both
// read "go", not a full path.
func commandPhaseName(cmd *exec.Cmd) string {
	if cmd.Path != "" {
		return filepath.Base(cmd.Path)
	}
	if len(cmd.Args) > 0 {
		return filepath.Base(cmd.Args[0])
	}
	return ""
}

// ensurePhase sets phase only when the task has none yet, so Run never
// overwrites a phase the caller already declared before calling it.
func (t *TaskHandle) ensurePhase(phase string) {
	if phase == "" || t.Snapshot().Phase != "" {
		return
	}
	t.Phase(phase)
}

// teeSubprocessWriter adds extra to existing without discarding it — an
// already-wired cmd.Stdout/cmd.Stderr keeps receiving output, and Run's own
// capture/phase plumbing observes the same bytes.
func teeSubprocessWriter(existing io.Writer, extra io.Writer) io.Writer {
	if existing == nil {
		return extra
	}
	return io.MultiWriter(existing, extra)
}
