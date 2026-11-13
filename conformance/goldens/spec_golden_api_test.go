package goldens_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestSpecP1_CollectionEach_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	branches := []string{"feat/a", "feat/b", "feat/c"}
	g := out.Group("branches")
	g.Summary("3 deleted")
	for name, task := range g.Each(branches) {
		n := name
		task.Delete("branch", func() error {
			_ = n
			return nil
		})
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapseFields(buf.String())
	if !strings.Contains(got, "✓ branches") {
		t.Fatalf("want aggregate Done parent, got:\n%s", buf.String())
	}
	if strings.Contains(got, "✓ feat/a") || strings.Contains(got, "✓ feat/b") {
		t.Fatalf("successful Each children must stay collapsed, got:\n%s", buf.String())
	}
}

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
	for name, task := range g.Each([]string{"feat/old-billing"}) {
		n := name
		task.Delete("branch", func() error {
			_ = n
			called = true
			return nil
		})
	}
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
	seq.Task("scan").Define(func() error { return nil })
	seq.Task("venv").Define(func() error { return nil })
	install := seq.Task("install")
	install.Define(func() error {
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

	worktrees.Task("wt-a").Define(func() error {
		close(wtStarted)
		<-wtRelease
		return nil
	})
	branches.Task("br-a").Define(func() error {
		close(brStarted)
		<-brRelease
		return nil
	})
	out.Task("fetch").After(worktrees, branches).Define(func() error {
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
