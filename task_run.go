package evo

import (
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run is the ordinary way to shell out from a Task: it executes cmd as this
// task's subprocess, wiring cmd.Stdout/cmd.Stderr through the same Evidence +
// PhaseWriter plumbing PhaseWriter uses directly. Each line becomes the
// task's live Phase, and every byte is retained (redacted, bounded) in the
// task's Evidence ring so DetailTail has proof after Fail — reach for
// Evidence directly only when the caller isn't running an *exec.Cmd. If
// cmd.Stdout/cmd.Stderr already point somewhere (a caller wiring its own log
// file, say), Run tees into it rather than replacing it.
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
//	    return task.Failf("build failed: %w", err)
//	}
//	task.Done()
func (t *TaskHandle) Run(cmd *exec.Cmd) error {
	if t != nil && t.out != nil {
		t.ensurePhase(commandPhaseName(cmd))
		pw := &phaseWriter{task: t, evidence: t.Evidence()}
		cmd.Stdout = teeSubprocessWriter(cmd.Stdout, pw)
		cmd.Stderr = teeSubprocessWriter(cmd.Stderr, pw)
	}
	return cmd.Run()
}

// shellWrapperBasenames names common shell/interpreter wrappers whose own
// basename is a meaningless phase — the actual work lives in a -c/-Command
// script argument, not the wrapper's own name. FP-004 ("no placeholder
// phase") applies to Run itself: exec.Command("sh", "-c", "go build ./...")
// must not publish "sh" as the live phase (beginner-11).
var shellWrapperBasenames = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "ksh": true, "dash": true,
	"cmd": true, "cmd.exe": true, "powershell": true, "powershell.exe": true, "pwsh": true,
}

// commandPhaseName is the human phase text for a not-yet-run *exec.Cmd: the
// basename of the resolved path when set, else the first argv element —
// exec.Command("go", "build") and exec.Command("/usr/bin/go", "build") both
// read "go", not a full path. A shell wrapper's own basename is skipped in
// favor of its -c/-Command script's first word; when neither is available,
// commandPhaseName returns "" and ensurePhase defers to first output rather
// than publish a placeholder.
func commandPhaseName(cmd *exec.Cmd) string {
	name := ""
	switch {
	case cmd.Path != "":
		name = filepath.Base(cmd.Path)
	case len(cmd.Args) > 0:
		name = filepath.Base(cmd.Args[0])
	}
	if name == "" || !shellWrapperBasenames[name] {
		return name
	}
	if script := shellScriptCommandName(cmd.Args); script != "" {
		return script
	}
	return ""
}

// shellScriptCommandName returns the basename of the first word of a shell
// -c (or Windows -Command) script argument, so "sh -c 'go build ./...'"
// reads "go", not "sh".
func shellScriptCommandName(args []string) string {
	for i, a := range args {
		if (a == "-c" || a == "-Command") && i+1 < len(args) {
			fields := strings.Fields(args[i+1])
			if len(fields) > 0 {
				return filepath.Base(fields[0])
			}
		}
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
