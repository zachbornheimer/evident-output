package evo

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// failedFileWithProblemRow is the one row every mode must draw for a failed
// File whose Task also carries Problems: a single evidence tree, Problems
// first and then the File's per-attribute branches, closed on the last
// branch only so the attributes never read as siblings of a finished tree.
const failedFileWithProblemRow = "✗ install hook  hook not installed\n" +
	"   ├─ hooks/  hooks are optional\n" +
	"   ├─ hook.sh\n" +
	"   │ failed: permissions\n" +
	"   ├─ contents  verified\n" +
	"   └─ ✗ permissions\n" +
	"        error  operation not permitted\n" +
	"        path   hook.sh\n" +
	"        mode   0644\n"

func declareFailedFileWithProblem(out *Run, spec FileSpec) {
	out.Task("install hook").Define(func(ctx context.Context) error {
		if err := File(ctx, spec); err != nil {
			return Fail("hook not installed", On("hook.sh"), Detail("failed: permissions"))
		}
		return nil
	}).Problem("hooks are optional", On("hooks/"))
}

// TestFailedFileWithProblemDrawsOneEvidenceTree pins that a failed File whose
// Task also has a Problem draws one evidence tree in Plain, Human piped and
// Human on a terminal: the tree closes with └─ once, on its last branch.
func TestFailedFileWithProblemDrawsOneEvidenceTree(t *testing.T) {
	newRun := func(t *testing.T, cfg Config, tune tuning) (*Run, FileSpec) {
		dir := t.TempDir()
		t.Chdir(dir)
		spec, fsys := newFailedChmodFile(t, dir, "hook.sh")
		spec.Mode = 0o644
		cfg.State = State{Dir: t.TempDir()}
		cfg.Facades.FS = fsys
		out := initRun(cfg, tune)
		t.Cleanup(func() { _ = out.close() })
		return out, spec
	}

	t.Run("plain", func(t *testing.T) {
		var buf bytes.Buffer
		out, spec := newRun(t, Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
		declareFailedFileWithProblem(out, spec)
		_ = out.finish()
		if got := buf.String(); !strings.HasPrefix(got, failedFileWithProblemRow) {
			t.Fatalf("plain mismatch:\n--- want prefix ---\n%s\n--- got ---\n%s", failedFileWithProblemRow, got)
		}
	})

	t.Run("human piped", func(t *testing.T) {
		var buf bytes.Buffer
		out, spec := newRun(t, Config{Output: Human, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
		declareFailedFileWithProblem(out, spec)
		_ = out.finish()
		if got := buf.String(); !strings.HasPrefix(got, failedFileWithProblemRow) {
			t.Fatalf("human piped mismatch:\n--- want prefix ---\n%s\n--- got ---\n%s", failedFileWithProblemRow, got)
		}
	})

	t.Run("human terminal", func(t *testing.T) {
		screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
		out, spec := newRun(t, Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: testkit.NewClock(), Terminal: screen}},
			tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
		declareFailedFileWithProblem(out, spec)
		_ = out.finish()
		if got := screen.PersistedText() + screen.FinalText(); !strings.Contains(got, failedFileWithProblemRow) {
			t.Fatalf("human terminal mismatch:\n--- want (contains) ---\n%s\n--- got ---\n%s", failedFileWithProblemRow, got)
		}
	})
}
