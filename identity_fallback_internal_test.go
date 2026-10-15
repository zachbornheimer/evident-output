package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestFailf_NoTitle_UsesExecutableBasename is I2: the synthetic
// Output-level failure/cancel task, when no Task name and no explicit
// Config.Title identify it, uses the caller's actual executable basename
// (via the processArgv0 facade) instead of the generic literal "command".
func TestFailf_NoTitle_UsesExecutableBasename(t *testing.T) {
	restore := processArgv0
	processArgv0 = func() string { return "/usr/local/bin/clean-repo" }
	defer func() { processArgv0 = restore }()

	var buf bytes.Buffer
	out := newOutput("", to(&buf), withNoColor(), plain())
	out.fail("boom")
	_ = out.finish()

	if !strings.Contains(buf.String(), "clean-repo") {
		t.Fatalf("expected the executable basename \"clean-repo\", got:\n%s", buf.String())
	}
}

// TestZeroConfigNameDefaultsToBinaryName pins that a zero Config.Name resolves once, in resolveConfig, to
// the executable's basename: the evo.run name is never null and the Plain band names the binary.
func TestZeroConfigNameDefaultsToBinaryName(t *testing.T) {
	restore := processArgv0
	processArgv0 = func() string { return "/usr/local/bin/clean-repo" }
	defer func() { processArgv0 = restore }()

	t.Run("evo.run name", func(t *testing.T) {
		var stdout bytes.Buffer
		out := newOutput("", wireFormatOption(JSON), wireStreamOption(&stdout), to(io.Discard))
		_ = out.finish()
		if got := out.decodedRunDocument(t).Name; got != "clean-repo" {
			t.Fatalf("evo.run name = %q, want %q", got, "clean-repo")
		}
	})
	t.Run("plain band", func(t *testing.T) {
		var human bytes.Buffer
		out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &human}}, tuning{color: colorNever})
		out.Task("branches").Define(effectOf(Delete, "stale local branch", 3))
		_ = out.finish()
		if !strings.Contains(human.String(), "clean-repo") {
			t.Fatalf("plain output does not name the binary %q:\n%s", "clean-repo", human.String())
		}
	})
}
