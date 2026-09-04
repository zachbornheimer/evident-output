package evo

import (
	"os/exec"
	"testing"
)

// TestCommandPhaseName_CompoundShellScriptHasNoInitialPhase is release-gate
// round 5 finding 7: a shell wrapper's -c script only names a meaningful
// initial phase when it is a single simple command. A compound script
// (newline, ;, &&, or |) has no one "first word" that honestly describes the
// whole script — commandPhaseName must return "" and let Run's ensurePhase
// defer to the child's first output line instead of publishing a misleading
// placeholder (e.g. "cd" for "cd /tmp && make").
func TestCommandPhaseName_CompoundShellScriptHasNoInitialPhase(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{"simple", "go build ./...", "go"},
		{"semicolon", "cd /tmp; make", ""},
		{"and-and", "cd /tmp && make", ""},
		{"pipe", "cat file | grep foo", ""},
		{"newline", "cd /tmp\nmake", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", tt.script)
			if got := commandPhaseName(cmd); got != tt.want {
				t.Fatalf("commandPhaseName(sh -c %q) = %q, want %q", tt.script, got, tt.want)
			}
		})
	}
}
