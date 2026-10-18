package evo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestPlainFoldsExcludedMembersSettlingAfterGrace pins CONTRACT.md §8 for a
// Group still open after its grace: a prune whose branches take real time
// streams its finished members, yet its Excluded members fold into the one
// tally line the Human final screen shows, not one row each.
func TestPlainFoldsExcludedMembersSettlingAfterGrace(t *testing.T) {
	const protected = 3
	var stderr lockedBuffer
	clock := testkit.NewClock()
	cfg := evo.Config{
		Name: "prune", Output: evo.Plain, State: evo.State{Dir: t.TempDir(), AppID: "contract"},
		Facades: evo.Facades{Stdout: &lockedBuffer{}, Stderr: &stderr, Stdin: strings.NewReader(""), Clock: clock},
	}
	slow := make(chan struct{})
	succeed := func(context.Context) error { return nil }
	done := make(chan evo.Result, 1)
	go func() {
		done <- evo.Execute(context.Background(), cfg, func(_ context.Context, r *evo.Run) error {
			branches := r.Group("branches")
			branches.Task("feature-fast").Define(succeed)
			for i := range protected {
				branches.Task(fmt.Sprintf("feature-protected-%d", i)).Define(func(context.Context) error {
					<-slow
					return evo.Exclude(excludedTallyReason)
				})
			}
			branches.Task("feature-slow").Define(func(context.Context) error {
				<-slow
				return nil
			})
			return nil
		})
	}()

	waitForStderr(t, &stderr, clock, "✓ feature-fast")
	close(slow)
	select {
	case <-done:
	case <-time.After(plainOrderDeadline):
		t.Fatalf("run did not finish; so far:\n%s", stderr.String())
	}

	text := stderr.String()
	lines := linesMentioning(text, excludedTallyReason)
	if len(lines) != 1 {
		t.Fatalf("%d lines name the reason %q, want one tally line:\n%s", len(lines), excludedTallyReason, text)
	}
	if !strings.Contains(lines[0], fmt.Sprint(protected)) {
		t.Errorf("tally line %q does not carry the count %d", lines[0], protected)
	}
	if strings.Contains(text, "feature-protected") {
		t.Errorf("an Excluded member printed its own row after the grace:\n%s", text)
	}
	if !strings.Contains(text, "✓ feature-slow") {
		t.Errorf("a member that succeeded after the grace stopped streaming:\n%s", text)
	}
	if tally, closing := strings.Index(text, lines[0]), strings.LastIndex(text, "✓ branches"); closing < 0 || tally < closing {
		t.Errorf("the tally must follow the block's closing row (tally at %d, closing row at %d):\n%s", tally, closing, text)
	}
}

// TestPlainKeepsALoneExcludedMemberSettlingAfterGrace is the other half of
// the fold: one Excluded item has no tally (the final output keeps its row),
// so a streamed block writes it as that row, before its closing row.
func TestPlainKeepsALoneExcludedMemberSettlingAfterGrace(t *testing.T) {
	var stderr lockedBuffer
	clock := testkit.NewClock()
	cfg := evo.Config{
		Name: "prune", Output: evo.Plain, State: evo.State{Dir: t.TempDir(), AppID: "contract"},
		Facades: evo.Facades{Stdout: &lockedBuffer{}, Stderr: &stderr, Stdin: strings.NewReader(""), Clock: clock},
	}
	slow := make(chan struct{})
	done := make(chan evo.Result, 1)
	go func() {
		done <- evo.Execute(context.Background(), cfg, func(_ context.Context, r *evo.Run) error {
			branches := r.Group("branches")
			branches.Task("feature-fast").Define(func(context.Context) error { return nil })
			branches.Task("feature-protected").Define(func(context.Context) error {
				<-slow
				return evo.Exclude(excludedTallyReason)
			})
			return nil
		})
	}()

	waitForStderr(t, &stderr, clock, "✓ feature-fast")
	close(slow)
	select {
	case <-done:
	case <-time.After(plainOrderDeadline):
		t.Fatalf("run did not finish; so far:\n%s", stderr.String())
	}

	text := stderr.String()
	row, closing := strings.Index(text, "○ feature-protected  "+excludedTallyReason), strings.LastIndex(text, "✓ branches")
	if row < 0 || closing < 0 || row > closing {
		t.Fatalf("a lone Excluded member must print its own row before the closing row (row at %d, closing at %d):\n%s", row, closing, text)
	}
	if strings.Contains(text, "excluded 1") {
		t.Errorf("one Excluded member has no tally:\n%s", text)
	}
}
