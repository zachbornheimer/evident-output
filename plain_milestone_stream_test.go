package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPlain_CollectionChildStreamsMilestones is the red-first proof for the
// canary's silent pipe: `zq clean-repo | cat` showed the header line and
// then nothing at all for the whole ~70s classify, with the finished rows
// arriving only at the end. The subject under test is the shape that run
// used — an explicitly declared child inside a collection, driven by
// Doing/Progress — which emitTaskRunningProgressiveLocked skipped outright
// (`st.collection != nil && !st.fromEach`), so the renderer, not the call
// site, is what went quiet.
//
// The dialect: plain streams one durable line per milestone crossed, thinned
// to about ten across a run, first and final always, with the static Running
// face — never silent between the first line and Done.
func TestPlain_CollectionChildStreamsMilestones(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, Title: "zq", Stdout: &buf,
	})

	classify := out.Group("worktrees").Task("classify")
	const total = 111
	for i := 1; i <= total; i++ {
		classify.Progress(i, total)
	}
	midRun := buf.String()
	classify.Done("111 worktrees")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if strings.TrimSpace(midRun) == "" {
		t.Fatalf("plain must never be silent while work is Running; whole run:\n%s", buf.String())
	}
	lines := strings.Split(strings.TrimRight(midRun, "\n"), "\n")
	if len(lines) < 5 || len(lines) > 20 {
		t.Fatalf("want ~10 thinned milestones, got %d:\n%s", len(lines), midRun)
	}
	if !strings.Contains(lines[0], "1/111") {
		t.Fatalf("the first tick always streams, got %q", lines[0])
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, fmt.Sprintf("%d/%d", total, total)) {
		t.Fatalf("the final n/n always streams, got %q", last)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "◐ worktrees  classify") {
			t.Fatalf("want the static Running face and the subject it belongs to, got %q", line)
		}
		if strings.Contains(line, "\x1b[") || strings.ContainsAny(line, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
			t.Fatalf("plain carries no ANSI and no spinner frames, got %q", line)
		}
	}
}
