package evo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestFinish_ReadmeQuickstart_EachLoopAutoResolvesDone is beginner-gate-2
// finding (i): the README quickstart's `Default().Task("install").Each(packages)`
// loop with no following Done. A completed Each loop sealed its absolute
// progress at total/total and told an honest, complete story — exactly the
// same easiest-path amnesty already given to a recorded mutation-verb
// effect — so it must not read as misuse.
func TestFinish_ReadmeQuickstart_EachLoopAutoResolvesDone(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	succeed(out.Task("working tree"))
	out.Task("branches").block(
		"local-only branch",
		Detail("commit or stash before continuing"),
	)
	out.Task("cleanup").Define(effectOf(Delete, "stale local branch", 2))
	install := out.Group("install")
	for _, pkg := range []string{"a", "b", "c"} {
		install.Task(pkg).Define(func(ctx context.Context) error { return nil })
	}

	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a sealed Each loop should auto-resolve Done)", err)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "misuse") {
		t.Fatalf("no misuse expected once the sealed loop auto-resolves:\n%s", rendered)
	}
	if code := out.conclusion().exitCode; code != exitRefused {
		// "branches" is deliberately Blocked in this quickstart-verbatim
		// scenario; the band must stay sane (Blocked → exit 1), not escalate.
		t.Fatalf("exit code = %d, want %d (ExitBlocked, the quickstart's own Block)", code, exitRefused)
	}
}

// TestFinish_TeachingLadder_EachThenReturnNil_NeverCancels is beginner-gate-2
// finding (ii): mcp/docs/teaching-ladder.md's standalone example is
// `Default().Task("scan").Each(items); return nil` verbatim — an uninterrupted run
// must conclude OK, never [cancelled]/130, since nothing ever signaled the
// process.
func TestFinish_TeachingLadder_EachThenReturnNil_NeverCancels(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	scan := out.Group("scan")
	for _, name := range []string{"one", "two"} {
		scan.Task(name).Define(func(ctx context.Context) error { return nil })
	}

	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a sealed Each loop should auto-resolve Done)", err)
	}
	if code := out.conclusion().exitCode; code != exitOK {
		t.Fatalf("exit code = %d, want %d (ExitOK) — a sealed Each loop must never read as cancelled", code, exitOK)
	}
	if state := out.conclusion().headline(); state == headlineCancelled {
		t.Fatalf("state = %v, want anything but StateCancelled on an uninterrupted run", state)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "misuse") {
		t.Fatalf("no misuse expected once the sealed loop auto-resolves:\n%s", rendered)
	}
}

// TestRun_BlockedWithLeftoverMisuse_NeverEscalatesToFailed exercises the
// same "Block → exit 1 wins" contract through the full Output.Run lifecycle
// (where Finish's returned misuse error previously forced ExitFailed even
// when the conclusion itself was Blocked): a genuinely blocked task plus an
// unrelated bookkeeping misuse (a double-resolve) must still leave the
// process exit code at ExitBlocked, per the documented "Block → exit 1"
// contract — bookkeeping misuse is not a reason to override the headline
// the run actually printed.
func TestRun_BlockedWithLeftoverMisuse_NeverEscalatesToFailed(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	code := out.run(context.Background(), func(ctx context.Context, r *Run) error {
		task := out.Task("branches")
		task.block("local-only branch")
		succeed(task) // already resolved — leftover bookkeeping misuse
		return nil
	}).ExitCode()

	if code != exitRefused {
		t.Fatalf("exit code = %d, want %d (ExitBlocked) — leftover misuse must not escalate the printed Block band to Failed:\n%s", code, exitRefused, buf.String())
	}
}
