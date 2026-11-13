package evo_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

var eachFailBoundParentFailedCount = regexp.MustCompile(`\d+\s+failed`)

const (
	eachFailBoundTotal      = 50
	eachFailBoundFailCount  = 40
	eachFailBoundParentName = "worktrees"
	eachFailBoundErrText    = "git status failed"
)

func eachFailBoundNames() []string {
	names := make([]string, eachFailBoundTotal)
	for i := range names {
		names[i] = fmt.Sprintf("wt-%03d", i)
	}
	return names
}

func eachFailBoundFailedNames(names []string) []string {
	return names[:eachFailBoundFailCount]
}

func eachFailBoundLiveOutput(screen *testkit.Screen) *evo.Output {
	delay := time.Duration(0)
	return evo.Init(evo.Config{
		Isolated:        true,
		Stdout:          io.Discard,
		Stderr:          io.Discard,
		Terminal:        screen,
		VisibilityDelay: &delay,
		MaxFrameRate:    1_000_000,
		Color:           evo.ColorNever,
	})
}

func countNamedAppearances(text string, names []string) (hit int, found []string) {
	for _, name := range names {
		if strings.Contains(text, name) {
			hit++
			found = append(found, name)
		}
	}
	return hit, found
}

func indexOfName(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

func firstLineContaining(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func assertEachFailBoundTTY(t *testing.T, label, text string, failedNames []string) {
	t.Helper()
	if !strings.Contains(text, eachFailBoundParentName) {
		t.Fatalf("%s: want parent name %q:\n%s", label, eachFailBoundParentName, text)
	}
	parentLine := firstLineContaining(text, eachFailBoundParentName)
	if !eachFailBoundParentFailedCount.MatchString(parentLine) {
		t.Fatalf("%s: finished parent must carry a failed count, parent line=%q\nfull:\n%s", label, parentLine, text)
	}
	if !strings.Contains(text, "not shown") {
		t.Fatalf("%s: want omission line with 'not shown':\n%s", label, text)
	}
	hit, found := countNamedAppearances(text, failedNames)
	if hit > 1 {
		t.Fatalf("%s: Each attention child names = %d (%v), want ≤1:\n%s", label, hit, found, text)
	}
	if hit < 1 {
		t.Fatalf("%s: want exactly one representative failed child name, got none:\n%s", label, text)
	}
}

// TestEach_FailWall_HeightIndependentBoundsTTY proves a tall Screen still
// collapses homogeneous Each Failures to one attention child + "N not shown".
// Height 100 defeats a height-budget cheat that would still paint dozens of ✗ rows.
func TestEach_FailWall_HeightIndependentBoundsTTY(t *testing.T) {
	t.Parallel()
	names := eachFailBoundNames()
	failedNames := eachFailBoundFailedNames(names)

	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.Height(100),
		testkit.NoColor(),
	)
	out := eachFailBoundLiveOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group(eachFailBoundParentName)
	for name, task := range g.Each(names) {
		n := name
		idx := indexOfName(names, n)
		if idx >= 0 && idx < eachFailBoundFailCount {
			task.Define(func() error { return errors.New(eachFailBoundErrText) })
			continue
		}
		task.Define(func() error { return nil })
	}

	live := screen.LatestLiveText()
	assertEachFailBoundTTY(t, "live", live, failedNames)

	_ = out.Finish()
	persisted := screen.PersistedText()
	assertEachFailBoundTTY(t, "persisted", persisted, failedNames)

	snap := g.Snapshot()
	if len(snap.Tasks) != eachFailBoundTotal {
		t.Fatalf("Snapshot tasks = %d, want %d", len(snap.Tasks), eachFailBoundTotal)
	}
	raw, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	jsonText := string(raw)
	for _, name := range names {
		if !strings.Contains(jsonText, name) {
			t.Fatalf("EncodeJSON missing child %q", name)
		}
	}

	var plainBuf bytes.Buffer
	plainOut := evo.Init(evo.Config{
		Isolated: true,
		Stdout:   &plainBuf,
		Plain:    true,
		Color:    evo.ColorNever,
		Clock:    testkit.NewClock(),
	})
	plainGroup := plainOut.Group(eachFailBoundParentName)
	for name, task := range plainGroup.Each(names) {
		n := name
		idx := indexOfName(names, n)
		if idx >= 0 && idx < eachFailBoundFailCount {
			task.Define(func() error { return errors.New(eachFailBoundErrText) })
			continue
		}
		task.Define(func() error { return nil })
	}
	_ = plainOut.Finish()
	assertEachFailBoundTTY(t, "plain Finish", plainBuf.String(), failedNames)
}

// TestEach_FailWall_DuringRunningKeepsProgressParent proves DURING paint
// keeps N/M + current child on the parent, surfaces ≤1 attention child, and
// does not synthesize a running-parent "· K failed" suffix.
func TestEach_FailWall_DuringRunningKeepsProgressParent(t *testing.T) {
	t.Parallel()
	names := eachFailBoundNames()
	failedNames := eachFailBoundFailedNames(names)
	runningName := names[eachFailBoundFailCount] // first success slot held Running

	screen := testkit.NewScreen(
		testkit.Interactive(),
		testkit.Width(80),
		testkit.Height(100),
		testkit.NoColor(),
	)
	out := eachFailBoundLiveOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	defer func() { <-done }()

	g := out.Group(eachFailBoundParentName)
	go func() {
		defer close(done)
		for name, task := range g.Each(names) {
			n := name
			idx := indexOfName(names, n)
			switch {
			case idx >= 0 && idx < eachFailBoundFailCount:
				task.Define(func() error { return errors.New(eachFailBoundErrText) })
			case n == runningName:
				task.Define(func() error {
					close(started)
					<-release
					return nil
				})
			default:
				task.Define(func() error { return nil })
			}
		}
	}()
	defer close(release)
	<-started

	deadline := time.Now().Add(2 * time.Second)
	var live string
	for {
		snap := g.Snapshot()
		failed := 0
		running := ""
		for _, child := range snap.Tasks {
			if child.State == evo.Failed {
				failed++
			}
			if child.State == evo.Running {
				running = child.Name
			}
		}
		live = screen.LatestLiveText()
		if failed >= 10 && running == runningName && strings.Contains(live, runningName) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for DURING gate (failed=%d running=%q):\n%s", failed, running, live)
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !strings.Contains(live, eachFailBoundParentName) {
		t.Fatalf("want parent name in DURING live:\n%s", live)
	}
	if !strings.Contains(live, fmt.Sprintf("/%d", eachFailBoundTotal)) {
		t.Fatalf("want N/%d progress on DURING parent:\n%s", eachFailBoundTotal, live)
	}
	parentLine := firstLineContaining(live, eachFailBoundParentName)
	if eachFailBoundParentFailedCount.MatchString(parentLine) {
		t.Fatalf("running parent must not synthesize failed count, parent line=%q\nfull:\n%s", parentLine, live)
	}
	if !strings.Contains(parentLine, runningName) {
		t.Fatalf("want current child %q on parent line %q\nfull:\n%s", runningName, parentLine, live)
	}

	hit, found := countNamedAppearances(live, failedNames)
	if hit > 1 {
		t.Fatalf("DURING Each attention names = %d (%v), want ≤1:\n%s", hit, found, live)
	}
}
