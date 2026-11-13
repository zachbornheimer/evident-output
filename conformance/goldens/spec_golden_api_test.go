package goldens_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestSpecP1_CollectionEach_Success pinned Each's own collapse-on-success
// aggregation: successful fromEach children stay invisible under one
// "✓ branches" parent row. 1.0 removed Each outright (§3.1: its
// get-or-create reliance is unsound) — a plain Group child now renders its
// own row regardless of outcome. Restoring a collapsed view for large
// homogeneous groups is renderer work for a later increment (§4:
// "aggregation is renderer-owned and automatic") — removed rather than
// pinning stale behavior.

func TestSpecP3_DryRunMutation_NeverCallsCallback(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		Title:    "retire",
		Stdout:   &buf,
		Plain:    true,
		Color:    evo.ColorNever,
		DryRun:   true,
		Clock:    testkit.NewClock(),
	})
	called := false
	g := out.Group("branches")
	g.Task("feat/old-billing").Delete("branch", func() error {
		called = true
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run mutation callback must not run")
	}
	got := collapseFields(buf.String())
	if !strings.Contains(got, "[planned]") || !strings.Contains(got, "delete") {
		t.Fatalf("want planned tense, got:\n%s", buf.String())
	}
	if strings.Contains(got, "[changed]") || strings.Contains(got, "deleted") {
		t.Fatalf("dry-run must not use committed tense, got:\n%s", buf.String())
	}
}

func TestSpecP4_SequenceDefine_DeclarationOrder(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "python", Stdout: &buf, Plain: true, Color: evo.ColorNever, Clock: testkit.NewClock()})
	seq := out.Sequence("python")
	seq.Task("scan").Define(func(ctx context.Context) error { return nil })
	seq.Task("venv").Define(func(ctx context.Context) error { return nil })
	install := seq.Task("install")
	install.Define(func(ctx context.Context) error {
		install.Done("14 modules")
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓ scan", "✓ venv", "✓ install  14 modules"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

func TestSpecAfter_FetchWaitsForGroups(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Title: "fetch", Stdout: bytes.NewBuffer(nil), Plain: true, Color: evo.ColorNever, Clock: testkit.NewClock()})
	t.Cleanup(func() { _ = out.Close() })

	worktrees := out.Group("worktrees")
	branches := out.Group("branches")
	wtRelease := make(chan struct{})
	brRelease := make(chan struct{})
	wtStarted := make(chan struct{})
	brStarted := make(chan struct{})
	fetchStarted := make(chan struct{})
	defer func() {
		select {
		case <-wtRelease:
		default:
			close(wtRelease)
		}
		select {
		case <-brRelease:
		default:
			close(brRelease)
		}
	}()

	worktrees.Task("wt-a").Define(func(ctx context.Context) error {
		close(wtStarted)
		<-wtRelease
		return nil
	})
	branches.Task("br-a").Define(func(ctx context.Context) error {
		close(brStarted)
		<-brRelease
		return nil
	})
	out.Task("fetch").After(worktrees, branches).Define(func(ctx context.Context) error {
		close(fetchStarted)
		return nil
	})

	<-wtStarted
	<-brStarted
	select {
	case <-fetchStarted:
		t.Fatal("fetch started before Group predecessors finished")
	default:
	}
	close(wtRelease)
	close(brRelease)
	<-fetchStarted
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func eachSkipNames(prefix string, n int) []string {
	items := make([]string, n)
	for i := 0; i < n; i++ {
		items[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return items
}
