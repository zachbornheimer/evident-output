package evo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// plainGraceStep moves the fake clock past the 500 ms a streamable Group
// keeps its first settled members to itself.
const plainGraceStep = 600 * time.Millisecond

// TestPlainStreamsALateMemberUnderItsOwnGroup is the golden for a Group that
// holds two Groups whose members settle interleaved: npm's "a" at once, pip's
// "a" and "b" next, npm's "b" last. The log attributes npm's late "b" to npm:
// it heads npm again after pip's lines instead of leaving "b" at member depth
// under pip.
func TestPlainStreamsALateMemberUnderItsOwnGroup(t *testing.T) {
	var stderr lockedBuffer
	clock := testkit.NewClock()
	cfg := evo.Config{
		Name: "nest", Output: evo.Plain, State: evo.State{Dir: t.TempDir(), AppID: "contract"},
		Facades: evo.Facades{Stdout: &lockedBuffer{}, Stderr: &stderr, Stdin: strings.NewReader(""), Clock: clock},
	}
	gates := map[string]chan struct{}{}
	for _, name := range []string{"pip.a", "pip.b", "npm"} {
		gates[name] = make(chan struct{})
	}
	member := func(gate string) func(context.Context) error {
		return func(context.Context) error {
			if gate != "" {
				<-gates[gate]
			}
			return nil
		}
	}
	done := make(chan evo.Result, 1)
	go func() {
		done <- evo.Execute(context.Background(), cfg, func(_ context.Context, r *evo.Run) error {
			clean := r.Group("clean caches")
			npm, pip := clean.Group("npm"), clean.Group("pip")
			npm.Task("a").Define(member(""))
			npm.Task("b").Define(member("npm"))
			pip.Task("a").Define(member("pip.a"))
			pip.Task("b").Define(member("pip.b"))
			return nil
		})
	}()

	waitForStderr(t, &stderr, clock, "✓ a")
	close(gates["pip.a"])
	waitForStderr(t, &stderr, clock, "pip\n      ✓ a")
	close(gates["pip.b"])
	waitForStderr(t, &stderr, clock, "✓ pip")
	close(gates["npm"])
	select {
	case <-done:
	case <-time.After(plainOrderDeadline):
		t.Fatalf("run did not finish; so far:\n%s", stderr.String())
	}

	want := "◐ clean caches\n" +
		"   ◐ npm\n" + "      ✓ a\n" +
		"   ◐ pip\n" + "      ✓ a\n" + "      ✓ b\n" +
		"   ✓ pip\n" +
		"   ◐ npm\n" + "      ✓ b\n" + "   ✓ npm\n" +
		"✓ clean caches\n"
	if got := stderr.String(); !strings.HasPrefix(got, want) {
		t.Fatalf("npm's late member is not attributed to npm:\n%s\nwant it to start with:\n%s", got, want)
	}
}

// waitForStderr advances the fake clock past a Group's grace until stderr
// holds want, and fails the test if it never does.
func waitForStderr(t *testing.T, stderr *lockedBuffer, clock *testkit.Clock, want string) {
	t.Helper()
	deadline := time.After(plainOrderDeadline)
	for !strings.Contains(stderr.String(), want) {
		select {
		case <-deadline:
			t.Fatalf("stderr never held %q; so far:\n%s", want, stderr.String())
		case <-time.After(2 * time.Millisecond):
			clock.Advance(plainGraceStep)
		}
	}
}
