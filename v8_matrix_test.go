// Package evo_test hosts the v8 mockup goldens: each of the terminal tabs
// the coordinator transcribed from evident-output-ui-v8.html, encoded as a
// scripted runtime model on Plain/testkit output and diffed exactly against
// the transcribed frame. Where the frame and the existing renderer
// disagreed, the renderer was fixed and the red diff is quoted in the
// commit body — this file's own comments call out the cases where the
// mockup and the normative spec text disagree with each other, and why the
// spec wins.
package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestV8_DryRunPlanOnly is the golden for the "Dry-run (plan-only)" tab,
// held to the contract §18 dry-run fixture: a Config.Subject header, three
// category Groups whose own Task classifies, summarizes and owns the
// Effect, one Skipped child Task per policy-excluded candidate, and a
// three-section [planned] ledger.
//
// The header carries evo's own "[dry-run]" tag before the subject (§27's
// worked example: "[dry-run] repo  ~/Developer/zq"). Policy-excluded items
// are Skipped and never warn, so the run concludes a pure [planned], and
// under the [dry-run] Subject header that verdict prints no band at all.
func TestV8_DryRunPlanOnly(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, DryRun: true, Color: evo.ColorNever, Plain: true,
		Subject: "zq prune  ~/Developer/Software-Automation-Holdings/.worktrees/eapp-system-style-contract-heading",
		Stdout:  &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	checkedOut, unpushed, protected := evo.Reason("checked out"), evo.Reason("unpushed"), evo.Reason("protected")
	dirty, ignored := evo.Reason("dirty"), evo.Reason("ignored files")
	categories := out.Group("categories")
	for _, category := range []*evo.TaskHandle{
		pruneCategory{
			name: "branches", summary: "459 checked",
			effect:  &evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 40},
			skipped: skippedItems("branch", reasonCount{checkedOut, 283}, reasonCount{unpushed, 135}, reasonCount{protected, 1}),
		}.declare(categories),
		pruneCategory{
			name: "worktrees", summary: "294 checked",
			effect:  &evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 1},
			skipped: skippedItems("worktree", reasonCount{dirty, 163}, reasonCount{unpushed, 89}, reasonCount{ignored, 40}),
		}.declare(categories),
		pruneCategory{
			name: "remote-tracking", summary: "4 stale refs",
			effect: &evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/*", Quantity: 4},
		}.declare(categories),
	} {
		if err := category.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := "[dry-run] zq prune  ~/Developer/Software-Automation-Holdings/.worktrees/eapp-system-style-contract-heading\n" +
		"\n" +
		"✓ branches         459 checked\n" +
		"  - skipped 419 (283 checked out, 135 unpushed, 1 protected)\n" +
		"✓ worktrees        294 checked\n" +
		"  - skipped 292 (163 dirty, 89 unpushed, 40 ignored files)\n" +
		"✓ remote-tracking  4 stale refs\n" +
		"\n" +
		"[planned] branches         delete 40 local tips\n" +
		"[planned] worktrees        remove 1 worktree\n" +
		"[planned] remote-tracking  delete 4 stale origin/*\n"
	if got := buf.String(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// reasonCount is one Skipped reason and how many candidates it excluded.
type reasonCount struct {
	reason evo.TaxonomyReason
	count  int
}

// skippedItems is count candidates per reason, named prefix-1, prefix-2,
// ..., each Skipped for its reason.
func skippedItems(prefix string, counts ...reasonCount) []skippedItem {
	var items []skippedItem
	for _, rc := range counts {
		for range rc.count {
			items = append(items, skippedItem{fmt.Sprintf("%s-%d", prefix, len(items)+1), rc.reason})
		}
	}
	return items
}

// TestV8_NothingToClean is the golden for the "Nothing to clean" tab: three
// checked subjects, none with any effect, and a closing summary line. The
// one policy-excluded branch is a Skipped child that folds under its
// category's row (§13), so nothing warns and the band reads a plain
// [ready]. The closing "prune  nothing to clean" line is the application's
// own Println of its verdict, layered on top of evo's conclusion band.
//
// Getting the per-item Skipped fold onto "branches" (contract §18's "own
// Task" shape) requires a Group (pruneCategory.declare's own nested
// per-category Group, not the shared "categories" parent), unlike the
// base (pre-1.1) version's flat standalone Tasks. A Group's own rows are
// batched and only render at Finish, while Println is a direct, immediate
// write — so an application Println called (as here) after the categories
// are declared but before Finish genuinely prints ahead of the category
// rows in the real byte stream, not merely in this golden. Calling Finish
// first would change what the test proves, not just its byte order — a
// real CLI prints its own closing line before Finish/exit, the same
// sequence this golden pins.
func TestV8_NothingToClean(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true, Title: "prune",
		Subject: "zq prune  ~/Developer/Personal/zq",
		Stdout:  &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	categories := out.Group("categories")
	branchesTask := pruneCategory{name: "branches", summary: "1 checked", skipped: skippedItems("branch", reasonCount{evo.Reason("protected"), 1})}.declare(categories)
	worktreesTask := pruneCategory{name: "worktrees", summary: "nothing to clean"}.declare(categories)
	remoteTrackingTask := pruneCategory{name: "remote-tracking", summary: "nothing to clean"}.declare(categories)
	for _, task := range []*evo.TaskHandle{branchesTask, worktreesTask, remoteTrackingTask} {
		if err := task.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	out.Println("prune  nothing to clean")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	// See the doc comment above: Println commits durably before Finish's
	// batch pass ever renders the categories Group's rows (Skipped child
	// folded under "branches" with no warned band), so this golden pins
	// Println ahead of the category rows, then the plain [ready] band —
	// the real order a CLI that prints its verdict before Finish produces.
	got := buf.String()
	want := "zq prune  ~/Developer/Personal/zq\n" +
		"prune  nothing to clean\n" +
		"✓ branches         1 checked\n" +
		"  - skipped 1 (protected)\n" +
		"✓ worktrees        nothing to clean\n" +
		"✓ remote-tracking  nothing to clean\n" +
		"\n[ready]  prune\n"
	if got != want {
		t.Fatalf("frame mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// newFailedChmodFile returns a FileSpec/testkit.FileFS pair that reconciles
// for real (a genuine temp-directory write) and then fails only the chmod
// step with an EPERM-shaped error — the fixture both TestV8_PartialFailure
// and TestV8_Stress script evo.File's chmod failure through, per spec
// §8.2's worked example: contents write cleanly, permissions alone fails.
// displayPath is deliberately the frame's own "~/Library/LaunchAgents/..."
// text — a relative FileSpec.Path resolves against the workspace directory
// (spec §8.1), so t.Chdir(dir) below makes that exact, unmodified string
// the real resolved path too, with no separate "pretty vs. real path"
// translation for the renderer to invent.
func newFailedChmodFile(t *testing.T, dir, displayPath string) (evo.FileSpec, *testkit.FileFS) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(displayPath)), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	fsys := testkit.NewFileFS()
	fsys.FailChmod(filepath.Join(dir, displayPath), errors.New("operation not permitted"))
	spec := evo.FileSpec{Path: displayPath, Contents: []byte("<plist/>"), Mode: 0o644}
	return spec, fsys
}

// TestV8_PartialFailure is the golden for the "Partial failure" tab: one
// evo.File reconciliation whose contents write succeeds and whose chmod
// then fails, rendering the spec §2/§8.2/§20-21/§41 per-attribute
// verification-detail shape — a satisfied attribute beside a failed one,
// the failed one's own error/path/mode Facts nested beneath it.
func TestV8_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	displayPath := "~/Library/LaunchAgents/com.acme.prod.agent.plist"
	spec, fsys := newFailedChmodFile(t, dir, displayPath)

	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true, FileFS: fsys,
		// StateDir isolates this test's manifest to its own temp
		// directory (spec §11.3) — without it, evo.File's default
		// manifest path is derived from the real machine cache dir and
		// can contend with any other concurrently running evo.File caller.
		StateDir: t.TempDir(),
		Stdout:   &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	agent := out.Task("write launch agent")
	agent.Define(func(ctx context.Context) error { return evo.File(ctx, spec) })

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := "✗ write launch agent  failed: permissions\n" +
		"  - contents  already satisfied\n" +
		"  ✗ permissions\n" +
		"    error  operation not permitted\n" +
		"    path   " + displayPath + "\n" +
		"    mode   0644\n"
	if got := buf.String(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestV8_Stress is the golden for the "Stress" tab's one genuinely new
// piece: TestV8_PartialFailure's verification-detail block, this time
// composed alongside pieces every other tab already proves on their own —
// a resolved Group, a standalone Done task with a committed Changes ledger
// entry, and the failed evo.File task. The frame's live bars/timers/
// warning-under-Running-child shapes are exactly TestV8_LiveParallelPrune's
// and the appendix-H live tests' own proven territory, not re-asserted
// here — recombining already-proven durable rows would test the same
// rendering paths those goldens already pin; only the verification block's
// composition alongside a resolved Group and a ledger is new.
func TestV8_Stress(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	displayPath := "~/Library/LaunchAgents/com.acme.prod.agent.plist"
	spec, fsys := newFailedChmodFile(t, dir, displayPath)

	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true, FileFS: fsys,
		StateDir: t.TempDir(),
		Stdout:   &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	deploy := out.Group("deploy production")
	succeed(deploy.Task("discover"))
	succeed(deploy.Task("services"), "already satisfied")

	remotes := out.Task("remote-tracking")
	commit(remotes.Summary("4 stale refs"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/*", Quantity: 4})

	agent := out.Task("write launch agent")
	agent.Define(func(ctx context.Context) error { return evo.File(ctx, spec) })

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := "✓ remote-tracking  4 stale refs\n" +
		"✗ write launch agent  failed: permissions\n" +
		"  - contents  already satisfied\n" +
		"  ✗ permissions\n" +
		"    error  operation not permitted\n" +
		"    path   " + displayPath + "\n" +
		"    mode   0644\n" +
		"✓ discover\n" +
		"✓ services  already satisfied\n" +
		"\n" +
		"[changed] remote-tracking  deleted 4 stale origin/*\n" +
		"\n" +
		"[failed]\n" +
		"!  already mutated: 4 stale origin/* deleted\n"
	if got := buf.String(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestV8_CancelledAfterMutation is the golden for the "Cancelled after
// mutation" tab: one task completes with a committed effect before
// cancellation, one is interrupted mid-work, and one never starts.
//
// Two deliberate departures from the transcribed frame's exact shape:
//
//   - Row order. The frame lists branches (not started) first, then
//     worktrees (interrupted), then remote-tracking (done) — but durable
//     plain output is an append-only log: a row commits the instant it
//     resolves (testkit's own PersistedText doc comment), and "branches"
//     genuinely never resolves until Finish's unresolved-task sweep, after
//     everything that already finished. The spec's own §43 canonical
//     example orders the same shape the other way — "not started" trailing
//     an already-interrupted sibling — which is exactly what an
//     append-only model can produce; that ordering is followed here.
//   - The frame doesn't show it, but a committed effect must never
//     disappear once work is cut short (spec §15/§43: "Committed effects
//     remain... never implying rollback"), so the cancellation band adds
//     "! partial changes were applied before cancellation" (contract §15).
//     The note is derived from the committed Effects, not caller-authored,
//     and it does not repeat the ledger line above it.
//
// The frame's glyph for the interrupted row ("-") is not used here either:
// spec §43's own example and the glyph table (§41) both use "■" for
// Cancelled, distinct from "-" for NotStarted — the frame's own two rows
// use the same glyph for two different states, which the spec resolves.
func TestV8_CancelledAfterMutation(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true, Title: "prune",
		Subject: "zq prune --apply  ~/.../eapp-system-style-contract-heading",
		Stdout:  &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("branches") // never touched: still Pending when Cancel hits.
	worktrees := out.Task("worktrees")
	remotes := out.Task("remote-tracking")

	commit(remotes.Summary("4/4"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/*", Quantity: 4})

	worktrees.Cancel("interrupted")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := "zq prune --apply  ~/.../eapp-system-style-contract-heading\n" +
		"✓ remote-tracking  4/4\n" +
		"■ worktrees        interrupted\n" +
		"- branches         not started\n" +
		"\n" +
		"[changed] remote-tracking  deleted 4 stale origin/*\n" +
		"\n" +
		"[cancelled] prune\n" +
		"  ! partial changes were applied before cancellation\n"
	if got := buf.String(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestV8_AlreadySatisfied is the golden for the "Already satisfied" tab: a
// Group whose already-satisfied children carry the muted suffix spec §19
// describes, and one Fact nested under its owning child.
//
// One departure from the frame: the frame indents the "path" Fact one level
// deeper than its sibling child rows (nested specifically under "write
// launch agent"). The actual renderer's writeCollectionChild nests a
// child's own Facts/warnings/taxonomy at the same shared indent
// (problemTreeIndent) every collection child uses for its own row — a
// deliberate bounded-depth convention (this codebase's own structure limits
// cap nesting depth generally), not a per-parent-relative indent. Changing
// it to match the frame would deepen indentation for every collection
// child's nested Fact/warning/taxonomy line, a much larger behavior change
// than this single tab warrants; the golden follows the actual, tested
// convention.
func TestV8_AlreadySatisfied(t *testing.T) {
	// "deploy production" has no Summary, so its header is not a row, and
	// its zero-information children (discover, prepare hosts, services:
	// nothing to say, nothing changed) are hidden while other content
	// exists. The launch agent's "path" is a routine Task Fact: contract
	// §13/§21 hide it at normal verbosity (§21 labels this exact
	// "✓ write launch agent / path ..." shape its "Verbose example"), which
	// leaves the launch agent zero-information too. Under verbose it owns a
	// visible Fact and keeps its row; cleanup always keeps its Summary.
	cases := []struct {
		verbosity evo.Verbosity
		want      string
	}{
		{evo.VerbosityNormal, "✓ cleanup  nothing to do\n"},
		{evo.VerbosityVerbose, "✓ write launch agent  already satisfied\n" +
			"  path  ~/Library/LaunchAgents/com.acme.prod.agent.plist\n" +
			"✓ cleanup             nothing to do\n"},
	}
	for _, tc := range cases {
		if got := renderV8AlreadySatisfied(t, tc.verbosity); got != tc.want {
			t.Fatalf("verbosity %v mismatch:\n--- want ---\n%s\n--- got ---\n%s", tc.verbosity, tc.want, got)
		}
	}
}

func renderV8AlreadySatisfied(t *testing.T, verbosity evo.Verbosity) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Color: evo.ColorNever, Plain: true, Stdout: &buf, Stderr: io.Discard, Verbosity: verbosity})
	t.Cleanup(func() { _ = out.Close() })

	// Spec §19's suffix is ResolutionAlreadySatisfied, produced only by a
	// pre-Define Verify that already holds — never a caller-written
	// Done("already satisfied") summary, which would also fire if File/Exec
	// no-op'd after Define (the case §19 forbids applying the suffix to).
	alreadySatisfied := func(task *evo.TaskHandle) {
		t.Helper()
		task.Verify(func(context.Context) (bool, error) { return true, nil })
		task.Define(func(context.Context) error {
			t.Fatal("Define must not run once Verify reports already satisfied")
			return nil
		})
	}

	deploy := out.Group("deploy production")
	alreadySatisfied(deploy.Task("discover"))
	alreadySatisfied(deploy.Task("prepare hosts"))
	alreadySatisfied(deploy.Task("services"))
	launchAgent := deploy.Task("write launch agent")
	launchAgent.Fact("path", "~/Library/LaunchAgents/com.acme.prod.agent.plist")
	alreadySatisfied(launchAgent)
	succeed(deploy.Task("cleanup"), "nothing to do")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestV8_StressLive is the golden for the HTML "Stress case" Replay tab's
// live shape: a still-running Group with mixed done/running/failed children,
// a warning on a running child, and a real evo.File permissions failure.
// Spec wins vs the HTML where they disagree: elapsed only after 5s Running
// (this golden advances 8s, past that threshold); empty bar cells are
// spaces; the live region does not invent a [changed]/[planned] ledger
// unless LiveRegion itself paints one.
func TestV8_StressLive(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	displayPath := "~/Library/LaunchAgents/com.acme.prod.agent.plist"
	spec, fsys := newFailedChmodFile(t, dir, displayPath)

	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, FileFS: fsys,
		StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	deploy := out.Group("deploy production")
	commit(deploy.Task("discover"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 5})

	hosts := deploy.Task("prepare hosts")
	hosts.Doing("host-031")
	hosts.Progress(31, 100)

	services := deploy.Task("services")
	services.Doing("payments-api")
	services.Progress(14, 40)
	services.Problem("audit-stream rollout slower than baseline", evo.Severity(evo.SeverityWarning))

	agent := deploy.Task("write launch agent")
	agent.Define(func(ctx context.Context) error { return evo.File(ctx, spec) })
	if err := agent.Wait(); err == nil {
		t.Fatal("write launch agent: expected permissions failure")
	}

	// cleanup has committed its Effect and is still running when the frame
	// is captured: its callback parks on release until the test ends.
	cleanup := deploy.Task("cleanup")
	committed, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	cleanup.Define(func(ctx context.Context) error {
		cleanup.Doing("feat/cleanup…")
		cleanup.Progress(7, 18)
		cleanup.Problem("origin remote slow to respond, retrying", evo.Severity(evo.SeverityWarning))
		err := evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/*", Quantity: 12},
			func(context.Context) error { return nil })
		close(committed)
		<-release
		return err
	})
	<-committed

	clock.Advance(8 * time.Second)
	cleanup.Progress(7, 18)

	// Departures from the HTML Replay frame, all because the spec wins:
	//   - unresolved Group header carries "N/M complete" (spec §18) plus
	//     elapsed after 5s (spec §24); the HTML shows elapsed only.
	//   - empty bar cells are spaces (spec §23).
	//   - File verification Facts nest one level under the failed attribute
	//     (writeVerificationDetails), so path/mode appear under permissions
	//     rather than sharing the HTML's i2 indent with "error".
	//   - both Effects land in [changed]: Config.DryRun is run-wide, and a
	//     dry run would skip the chmod failure this golden needs. LiveRegion
	//     still projects s.Plans as [planned] when a dry-run run has them.
	glyph := firstRune(screen.LatestLiveText())
	want := glyph + " deploy production  1/5 complete — 8s\n" +
		"   ✓ discover\n" +
		"   " + glyph + " prepare hosts  [███         ]  31/100 — 8s\n" +
		"      " + glyph + " host-031\n" +
		"   " + glyph + " services   [████        ]  14/40 — 8s\n" +
		"      " + glyph + " payments-api\n" +
		"      ! audit-stream rollout slower than baseline\n" +
		"   ✗ write launch agent  failed: permissions\n" +
		"      - contents  already satisfied\n" +
		"      ✗ permissions\n" +
		"        error  operation not permitted\n" +
		"        path   " + displayPath + "\n" +
		"        mode   0644\n" +
		"   " + glyph + " cleanup    [████        ]  7/18 — 8s\n" +
		"      " + glyph + " feat/cleanup…\n" +
		"      ! origin remote slow to respond, retrying\n" +
		"\n" +
		"[changed] discover  deleted 5 local tips\n" +
		"[changed] cleanup   deleted 12 stale origin/*"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestV8_DependencyInstall is the golden for the "Dependency install" tab:
// one Running task's stable parent line (bar/count/timer) plus its one
// activity child, on the fake clock/screen.
func TestV8_DependencyInstall(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever,
		// A high cap disables the interactive redraw frame-rate coalescing
		// (default 20fps/50ms) that would otherwise drop the second of two
		// back-to-back Doing/Progress calls made at the same fake-clock
		// instant, and this golden asserts the frame that follows both.
		MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install dependencies")
	install.Doing("urllib3")
	install.Progress(14, 40)
	clock.Advance(7 * time.Second)
	install.Progress(14, 40) // re-render at the advanced clock for the timer.

	// Two departures from the frame's literal spacing/indent, both matching
	// established, already-tested conventions elsewhere rather than this
	// one mockup's exact characters:
	//   - one space before the elapsed suffix ("14/40 — 7s"), not two —
	//     heartbeatSuffix's own " — <elapsed>" format, shared by every
	//     other elapsed-suffix golden in this suite.
	//   - the activity child indents 3 spaces, matching every other child
	//     row's indent (writeLiveTaskLine's pad), not the frame's 2.
	//
	// The spinner glyph is whichever frame the shared animation clock lands
	// on (spec §23.1: motion only proves liveness, no specific frame is
	// normative) — not necessarily the mockup's illustrative "⠋".
	glyph := firstRune(screen.LatestLiveText())
	want := glyph + " install dependencies  [████        ]  14/40 — 7s\n" +
		"   " + glyph + " urllib3"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// firstRune returns s's first rune as a string — used to pin a golden's
// asserted spinner glyph to whichever frame the shared animation clock
// actually produced, since the exact frame is decorative, not normative.
func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// TestV8_LiveParallelPrune is the golden for the "Live parallel prune" tab:
// three standalone Running siblings, each with its own stable parent line
// (bar/count/timer) and one activity child, name-column aligned.
//
// One deliberate departure: the frame shows the elapsed suffix at "— 2s",
// but spec §24 is explicit that "Elapsed time appears automatically after
// 5 seconds of actual Running time" — the existing, already-tested
// elapsedAfter threshold (internal/render/live.go) matches the spec's own
// normative text, not the frame's illustrative "2s". This golden advances
// the clock 5s instead.
func TestV8_LiveParallelPrune(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	worktrees := out.Task("worktrees")
	remotes := out.Task("remote-tracking")

	branches.Doing("feat/style-contract")
	branches.Progress(120, 459)
	worktrees.Doing("eapp-system-style-contract-heading")
	worktrees.Progress(70, 294)
	remotes.Doing("origin/old-style")
	remotes.Progress(1, 4)
	clock.Advance(5 * time.Second)
	remotes.Progress(1, 4)

	glyph := firstRune(screen.LatestLiveText())
	// Bar fill is proportional to completed/total (spec §23: "the bar is
	// decorative", the count is authoritative) — 70/294 and 1/4 round to
	// fewer filled cells than the frame's illustrative bars.
	want := glyph + " branches         [███         ]  120/459 — 5s\n" +
		"   " + glyph + " feat/style-contract\n" +
		glyph + " worktrees        [██          ]  70/294 — 5s\n" +
		"   " + glyph + " eapp-system-style-contract-heading\n" +
		glyph + " remote-tracking  [███         ]  1/4 — 5s\n" +
		"   " + glyph + " origin/old-style"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestV8_GenericSuccessPlusActiveWork is the golden for the "Generic clean
// success + active work" tab: a fully-resolved Group alongside a still-
// Running standalone task, both visible in the same live frame.
//
// Matches the frame and spec §18's own worked example for this exact
// three-child "launch agent" case exactly: once every child has settled, the
// header carries no count suffix at all ("✓ launch agent", not "✓ launch
// agent  3/3 complete") — writeLiveCollection only shows "N/total complete"
// while the group is still unresolved, the same way an unresolved row's
// count is diagnostic and a resolved row's ✓ glyph already says "done".
func TestV8_GenericSuccessPlusActiveWork(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	agent := out.Group("launch agent")
	succeed(agent.Task("write plist"))
	succeed(agent.Task("register"))
	succeed(agent.Task("start"))

	install := out.Task("install dependencies")
	install.Doing("requests")
	install.Progress(18, 40)
	clock.Advance(6 * time.Second)
	install.Progress(18, 40)

	glyph := firstRune(strings.TrimPrefix(screen.LatestLiveText(), "✓ write plist\n✓ register\n✓ start\n"))
	want := "✓ write plist\n" +
		"✓ register\n" +
		"✓ start\n" +
		glyph + " install dependencies  [█████       ]  18/40 — 6s\n" +
		"   " + glyph + " requests"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
