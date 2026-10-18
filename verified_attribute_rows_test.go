package evo

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestVerifiedAttributeRowsReadAsVerification pins how a File's per-attribute
// outcomes read: a satisfied attribute is a branch of its Task's row that says
// "verified", never a NotStarted sibling that says "already satisfied". The
// attribute may have been written this very run, and CONTRACT §8 forbids a
// projection inventing state another projection lacks; "already satisfied"
// belongs to the Satisfied outcome alone.
func TestVerifiedAttributeRowsReadAsVerification(t *testing.T) {
	displayPath := "~/Library/LaunchAgents/com.acme.prod.agent.plist"
	const failedFacts = "        error  operation not permitted\n" +
		"        path   ~/Library/LaunchAgents/com.acme.prod.agent.plist\n" +
		"        mode   0644\n"

	// Settlement order between sibling roots is a scheduling matter, so the
	// Group child and the root Task render in separate runs.
	for _, tc := range []struct {
		name  string
		place func(*Run) *Task
		want  string
	}{
		{
			"plain verbose success under a Group", func(r *Run) *Task { return r.Group("g").Task("alpha") },
			"✓ g\n   ✓ alpha\n      ├─ contents  verified\n      └─ permissions  verified\n",
		},
		{
			"plain verbose success at the root", func(r *Run) *Task { return r.Task("beta") },
			"✓ beta\n   ├─ contents  verified\n   └─ permissions  verified\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			var buf bytes.Buffer
			out := initRun(Config{Output: Plain, Verbose: true, State: State{Dir: t.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: &buf, FS: testkit.NewFileFS()}}, tuning{color: colorNever})
			t.Cleanup(func() { _ = out.close() })
			tc.place(out).Define(func(ctx context.Context) error {
				return File(ctx, FileSpec{Path: "written.md", Contents: []byte("hi"), Mode: 0o644})
			})
			if err := out.finish(); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			if strings.Contains(got, "already satisfied") {
				t.Fatalf("a File this run wrote must not read \"already satisfied\":\n%s", got)
			}
			if !strings.HasPrefix(got, tc.want) {
				t.Fatalf("verification rows mismatch:\n--- want prefix ---\n%s\n--- got ---\n%s", tc.want, got)
			}
		})
	}

	t.Run("plain failed File", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		spec, fsys := newFailedChmodFile(t, dir, displayPath)
		var buf bytes.Buffer
		out := initRun(Config{Output: Plain, State: State{Dir: t.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: &buf, FS: fsys}}, tuning{color: colorNever})
		t.Cleanup(func() { _ = out.close() })
		out.Task("write launch agent").Define(func(ctx context.Context) error { return File(ctx, spec) })
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}
		want := "✗ write launch agent  failed: permissions\n" +
			"   ├─ contents  verified\n" +
			"   └─ ✗ permissions\n" + failedFacts
		if got := buf.String(); !strings.HasPrefix(got, want) || strings.Contains(got, "already satisfied") {
			t.Fatalf("failed File rows mismatch:\n--- want prefix ---\n%s\n--- got ---\n%s", want, got)
		}
	})

	t.Run("human failed File under a Group, live and persisted", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		spec, fsys := newFailedChmodFile(t, dir, displayPath)
		screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
		out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: testkit.NewClock(), Terminal: screen, FS: fsys}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
		t.Cleanup(func() { _ = out.close() })
		agent := out.Group("deploy").Task("write launch agent")
		agent.Define(func(ctx context.Context) error { return File(ctx, spec) })
		if err := agent.wait(); err == nil {
			t.Fatal("expected the chmod failure")
		}
		want := "   ✗ write launch agent  failed: permissions\n" +
			"      ├─ contents  verified\n" +
			"      └─ ✗ permissions\n" +
			"           error  operation not permitted\n" +
			"           path   ~/Library/LaunchAgents/com.acme.prod.agent.plist\n" +
			"           mode   0644"
		if live := screen.LatestLiveText(); !strings.Contains(live, want) || strings.Contains(live, "already satisfied") {
			t.Fatalf("live frame mismatch:\n--- want (contains) ---\n%s\n--- got ---\n%s", want, live)
		}
		_ = out.finish()
		if persisted := screen.PersistedText(); strings.Contains(persisted, "already satisfied") || !strings.Contains(persisted, "├─ contents  verified") {
			t.Fatalf("persisted output mismatch:\n%s", persisted)
		}
	})
}
