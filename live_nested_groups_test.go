package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestLive_PerItemGroupsKeepHeaderAndAttention pins E-111: a Group of more
// per-item Groups than the frame has rows gave each nested Group a share
// of 0 rows, so each painted a bare "…  1 not shown" in place of its row,
// the outer header vanished, and the finished frame was 23 identical
// omission lines the repaint dedup then froze. The outer header now stays
// with its N/M count, the unfinished items fill the rows in attention
// order, and the finished ones fold into one omission line.
func TestLive_PerItemGroupsKeepHeaderAndAttention(t *testing.T) {
	const n, workers = 30, 4
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := evo.Init(evo.Config{
		Isolated: true, Clock: testkit.NewClock(), Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000, MaxConcurrency: workers,
	})
	t.Cleanup(func() { _ = out.Close() })

	items := out.Group("items")
	release, started := make([]chan struct{}, n), make([]chan struct{}, n)
	tasks := make([]*evo.TaskHandle, n)
	for i := range n {
		release[i], started[i] = make(chan struct{}), make(chan struct{})
		tasks[i] = items.Group(fmt.Sprintf("item %d", i)).Task("a").Define(func(context.Context) error {
			close(started[i])
			<-release[i]
			return nil
		})
	}
	const finished = 10
	for i := range finished {
		<-started[i]
		close(release[i])
		_ = tasks[i].Wait()
	}
	for i := finished; i < finished+workers; i++ {
		<-started[i]
	}

	mid := screen.LatestLiveText()
	glyph := spinnerOf(mid, fmt.Sprintf("item %d ", finished))
	var want strings.Builder
	fmt.Fprintf(&want, "%s items  %d/%d complete\n", glyph, finished, n)
	for i := finished; i < finished+workers; i++ {
		fmt.Fprintf(&want, "   %s item %d  a  working…\n", glyph, i)
	}
	for i := finished + workers; i < n; i++ {
		fmt.Fprintf(&want, "   ○ item %d  a\n", i)
	}
	fmt.Fprintf(&want, "   …  %d not shown", finished)
	if mid != want.String() {
		t.Errorf("mid-run frame:\n--- want ---\n%s\n--- got ---\n%s", want.String(), mid)
	}

	for i := finished; i < n; i++ {
		<-started[i]
		close(release[i])
		_ = tasks[i].Wait()
	}
	_ = items.Wait()
	if got, want := screen.LatestLiveText(), fmt.Sprintf("✓ items\n   …  %d not shown", n); got != want {
		t.Errorf("finished frame:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// spinnerOf is the glyph that starts the first row of frame naming row.
func spinnerOf(frame, row string) string {
	for line := range strings.SplitSeq(frame, "\n") {
		if strings.Contains(line, row) {
			return firstRune(strings.TrimLeft(line, " "))
		}
	}
	return ""
}

// TestNestedGroups_ThreeLevelPlainByteShape proves the plain/
// durable projection's byte shape for a Sequence nested three deep
// (release > python > venv > install task) — E2 proved the state derivation
// recurses correctly; this golden proves the rendered indentation does too.
func TestNestedGroups_ThreeLevelPlainByteShape(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	python := root.Sequence("python")
	venv := python.Sequence("venv")
	succeed(venv.Task("install"))

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	want := "✓ release\n" +
		"   ✓ python\n" +
		"      ✓ venv\n" +
		"         ✓ install\n"
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("want the 3-level nested container to indent 3 spaces per level, got:\n%s", buf.String())
	}
}

// TestNestedGroups_ThreeLevelLiveByteShape proves the same
// nesting depth in the interactive live region: each container level still
// indents its children 3 spaces deeper while Running.
func TestNestedGroups_ThreeLevelLiveByteShape(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	python := root.Sequence("python")
	venv := python.Sequence("venv")
	install := venv.Task("install")
	install.Doing("installing")

	frame := screen.LatestLiveText()
	lines := strings.Split(frame, "\n")
	var indentOf = func(line string) int {
		return len(line) - len(strings.TrimLeft(line, " "))
	}
	var releaseIndent, pythonIndent, venvIndent, installIndent int
	found := 0
	for _, line := range lines {
		switch {
		case strings.Contains(line, "release"):
			releaseIndent = indentOf(line)
			found++
		case strings.Contains(line, "python"):
			pythonIndent = indentOf(line)
			found++
		case strings.Contains(line, "venv"):
			venvIndent = indentOf(line)
			found++
		case strings.Contains(line, "install"):
			installIndent = indentOf(line)
			found++
		}
	}
	if found != 4 {
		t.Fatalf("want all 4 nesting levels present in the live frame, got %d:\n%s", found, frame)
	}
	if releaseIndent >= pythonIndent || pythonIndent >= venvIndent || venvIndent >= installIndent {
		t.Fatalf("want strictly increasing indent per nesting level (release=%d python=%d venv=%d install=%d):\n%s",
			releaseIndent, pythonIndent, venvIndent, installIndent, frame)
	}

	succeed(install)
	_ = out.Finish()
}
