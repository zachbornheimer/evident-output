package evo_test

import (
	"io"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

var unicodeSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func lineFor(live, name string) string {
	for _, line := range strings.Split(live, "\n") {
		if strings.Contains(line, name) {
			return line
		}
	}
	return ""
}

func hasSpinnerGlyph(s string) bool {
	for _, frame := range unicodeSpinnerFrames {
		if strings.Contains(s, frame) {
			return true
		}
	}
	return false
}

func TestLive_DeclaredToolTaskSpinsBeforeCheck(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("go@1.25.11")
	live := screen.LatestLiveText()
	row := lineFor(live, "go@1.25.11")
	if row == "" {
		t.Fatalf("declared task missing from live region:\n%s", live)
	}
	if strings.Contains(row, "✓") {
		t.Fatalf("tool row first appeared already complete:\n%s", row)
	}
	task.Doing("checking")
	live = screen.LatestLiveText()
	row = lineFor(live, "go@1.25.11")
	if !hasSpinnerGlyph(row) {
		t.Fatalf("submitted tool row has no spinner:\n%s", row)
	}

	clock.Advance(10 * time.Millisecond)
	task.Done("/usr/bin/go")
	after := screen.LatestLiveText() + "\n" + screen.PersistedText()
	if !strings.Contains(after, "✓") || !strings.Contains(after, "go@1.25.11") {
		t.Fatalf("after Done, check glyph missing:\nlive=%q\npersisted=%q", screen.LatestLiveText(), screen.PersistedText())
	}

	var sawName bool
	for _, op := range screen.Operations() {
		if op.Kind != "live" && op.Kind != "durable" {
			continue
		}
		if !strings.Contains(op.Text, "go@1.25.11") {
			continue
		}
		if !sawName {
			sawName = true
			if strings.Contains(op.Text, "✓") && !hasSpinnerGlyph(op.Text) {
				t.Fatalf("first frame with the tool name was already checked:\n%s", op.Text)
			}
		}
	}
	if !sawName {
		t.Fatal("no live/durable op named the tool")
	}
}

func TestLive_OneChildDisplayGroupIsSingleLine(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("run")
	child := g.Task("install:fresh-start")
	child.Doing("building")
	live := screen.LatestLiveText()
	if strings.Contains(live, "0/1 complete") || strings.Contains(live, "1/1 complete") {
		t.Fatalf("1-child group leaked N/M complete header:\n%s", live)
	}
	lines := nonemptyLines(live)
	if len(lines) != 1 {
		t.Fatalf("1-child group want 1 live line, got %d:\n%s", len(lines), live)
	}
	if !strings.Contains(lines[0], "install:fresh-start") {
		t.Fatalf("collapsed line missing the child name:\n%s", lines[0])
	}
}

func TestLive_FastBindSpinnerVisibleOnWallClock(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("go@1.25.11")
	task.Doing("resolving go@1.25.11")
	live := screen.LatestLiveText()
	row := lineFor(live, "go@1.25.11")
	if !hasSpinnerGlyph(row) {
		t.Fatalf("want spinner before Done on wall clock:\n%s", live)
	}
	task.Done("/usr/bin/go")
	after := screen.PersistedText()
	if !strings.Contains(after, "✓") || !strings.Contains(after, "go@1.25.11") {
		t.Fatalf("want check after hold:\n%s", after)
	}
}

func nonemptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
